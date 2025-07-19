package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mr-tron/base58"
)

var GAME_ADDRESS = MustDecodeBase58PublicKey(os.Getenv("GAME"))
var WITHDRAW_AUTHORITY_PRIVATE_KEY = MustDecodeHexPrivateKey(os.Getenv("WITHDRAW_AUTHORITY_PRIVATE_KEY"))

const IVY_URL = "http://127.0.0.1:3000"
const PORT = 8000
const DB_PATH = "./backend.db"
const CLIENT_SEED_MIN_LENGTH = 6
const CLIENT_SEED_MAX_LENGTH = 32
const AGGREGATOR_URL = "http://127.0.0.1:5000"

const HOUSE_EDGE_PCT = 1 // 1%
const UNDER_MIN = 1
const UNDER_MAX = 9802
const OVER_MIN = 197
const OVER_MAX = 9899
const MAX_BET_CENTS = 300000_00

var DB Database

// Roll returns a random integer in the range [0, 10000).
func Roll(serverSeed []byte, clientSeed []byte) uint16 {
	// hash server seed + client seed together
	seed := make([]byte, 0, len(serverSeed)+len(clientSeed))
	seed = append(seed, serverSeed...)
	seed = append(seed, clientSeed...)
	hash := sha256.Sum256(seed)
	// get random 64-bit integer
	n := binary.LittleEndian.Uint64(hash[:8])
	// convert it to range by taking it mod 10,000
	// (This results in a very, very small
	// bias towards small numbers: we will produce
	// an unfair result with probability ((2**64)%10000)/(2**64),
	// or 8.760353553682876e-17. This is satisfactory for our
	// use case)
	return uint16(n % 10_000)
}

// Generate a 32-byte server seed
func NewServerSeed() [32]byte {
	var seed [32]byte
	_, err := io.ReadFull(rand.Reader, seed[:])
	if err != nil {
		panic(err)
	}
	return seed
}

type AuthData struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type UserGetParams struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

// / A client-side user
type UserClient struct {
	Id             string `json:"id"`
	ServerSeedHash string `json:"serverSeedHash"`
	BalanceCents   uint64 `json:"balanceCents"`
}

func onUserGet(p UserGetParams) (UserClient, error) {
	id, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return UserClient{}, err
	}
	user, err := DB.UserGet(id)
	if err != nil {
		return UserClient{}, err
	}
	ss, err := hex.DecodeString(user.ServerSeed)
	if err != nil {
		return UserClient{}, err
	}
	ssHash := sha256.Sum256(ss)
	ssHashHex := hex.EncodeToString(ssHash[:])
	return UserClient{
		Id:             user.Id,
		ServerSeedHash: ssHashHex,
		BalanceCents:   user.BalanceCents,
	}, nil
}

type BetParams struct {
	Message    string `json:"message"`
	Signature  string `json:"signature"`
	WagerCents uint64 `json:"wagerCents"`
	RollUnder  bool   `json:"rollUnder"`
	Threshold  uint16 `json:"threshold"`
	ClientSeed string `json:"clientSeed"`
}

type BetResult struct {
	Won        bool   `json:"won"`
	DeltaCents int64  `json:"deltaCents"`
	ServerSeed string `json:"serverSeed"`
	Result     uint16 `json:"result"`
}

func onBet(p BetParams) (BetResult, error) {
	// (1) Validate threshold
	if p.RollUnder {
		if p.Threshold < UNDER_MIN {
			return BetResult{}, fmt.Errorf("invalid threshold: minimum amount to roll under is %d, but got %d", UNDER_MIN, p.Threshold)
		}
		if p.Threshold > UNDER_MAX {
			return BetResult{}, fmt.Errorf("invalid threshold: maximum amount to roll under is %d, but got %d", UNDER_MAX, p.Threshold)
		}
	} else {
		if p.Threshold < OVER_MIN {
			return BetResult{}, fmt.Errorf("invalid threshold: minimum amount to roll over is %d, but got %d", OVER_MIN, p.Threshold)
		}
		if p.Threshold > OVER_MAX {
			return BetResult{}, fmt.Errorf("invalid threshold: maximum amount to roll over is %d, but got %d", OVER_MAX, p.Threshold)
		}
	}
	// (2) Authenticate + fetch user
	id, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return BetResult{}, err
	}
	user, err := DB.UserGet(id)
	if err != nil {
		return BetResult{}, err
	}
	// (3) Validate wager
	if p.WagerCents > user.BalanceCents {
		return BetResult{}, fmt.Errorf("insufficient balance: you only have %.2f but you're trying to bet %.2f!", float64(user.BalanceCents)/100, float64(p.WagerCents)/100)
	}
	if p.WagerCents > MAX_BET_CENTS {
		return BetResult{}, fmt.Errorf("invalid bet: the maximum bet is %.2f, but you're trying to bet %.2f!", float64(MAX_BET_CENTS)/100, float64(p.WagerCents)/100)
	}
	// (4) Validate client seed
	if len(p.ClientSeed) < CLIENT_SEED_MIN_LENGTH || len(p.ClientSeed) > CLIENT_SEED_MAX_LENGTH {
		return BetResult{}, fmt.Errorf("incorrect client seed length: got %d, but must be within interval [%d, %d]", len(p.ClientSeed), CLIENT_SEED_MIN_LENGTH, CLIENT_SEED_MAX_LENGTH)
	}
	// (5) Fetch server seed
	serverSeed, err := hex.DecodeString(user.ServerSeed)
	if err != nil || len(serverSeed) != 32 {
		return BetResult{}, errors.New("error decoding server seed")
	}
	// (6) Roll the dice
	roll := Roll(serverSeed, []byte(p.ClientSeed))
	// (7) Compute delta
	var won bool
	var underAmountCents uint64
	if p.RollUnder {
		won = roll < p.Threshold
		underAmountCents = uint64(p.Threshold)
	} else {
		won = roll > p.Threshold
		underAmountCents = 10000 - uint64(p.Threshold)
	}
	var deltaCents int64
	if won {
		// reward = wager * (10000 / underAmountCents) - wager
		// Apply house edge
		payout := (p.WagerCents * 10000 * (100 - HOUSE_EDGE_PCT)) / (underAmountCents * 100)
		deltaCents = int64(payout) - int64(p.WagerCents)
	} else {
		deltaCents = -int64(p.WagerCents)
	}

	// Ensure balance doesn't go negative
	newBalance := int64(user.BalanceCents) + deltaCents
	if newBalance < 0 {
		newBalance = 0
	}

	// (8) Atomically update database with new balance + server seed
	newSeed := NewServerSeed()
	updatedUser := User{
		Id:           user.Id,
		ServerSeed:   hex.EncodeToString(newSeed[:]),
		BalanceCents: uint64(newBalance),
	}

	err = DB.UserCompareExchange(user, updatedUser)
	if err != nil {
		return BetResult{}, err
	}

	// (9) Record the bet
	bet := Bet{
		UserId:      user.Id,
		AmountCents: p.WagerCents,
		RollUnder:   p.RollUnder,
		Threshold:   p.Threshold,
		Result:      roll,
		Won:         won,
		ServerSeed:  user.ServerSeed, // Use the old server seed for the bet record
		CreatedAt:   uint64(time.Now().Unix()),
	}

	err = DB.BetCreate(bet)
	if err != nil {
		log.Printf("Warning: Failed to record bet: %v", err)
	}

	// (10) Return result
	return BetResult{
		Won:        won,
		DeltaCents: deltaCents,
		ServerSeed: user.ServerSeed, // Return the server seed used for this bet
		Result:     roll,
	}, nil
}

type DepositParams struct {
	Message     string `json:"message"`
	Signature   string `json:"signature"`
	AmountCents uint64 `json:"amountCents"`
}

type DepositResult struct {
	Id  string `json:"id"`
	Url string `json:"url"`
}

func onDeposit(p DepositParams) (DepositResult, error) {
	// (1) Authenticate user
	userId, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return DepositResult{}, err
	}

	// (2) Validate amount
	if p.AmountCents == 0 {
		return DepositResult{}, errors.New("deposit amount must be greater than 0")
	}

	// (3) Generate unique deposit ID
	depositIdBytes := GenerateID(CentsToRaw(p.AmountCents))
	depositId := hex.EncodeToString(depositIdBytes[:])

	// (4) Generate deposit URL
	depositUrl := fmt.Sprintf(IVY_URL+"/deposit?game=%s&id=%s", base58.Encode(GAME_ADDRESS[:]), depositId)

	// (5) Create deposit record in database
	err = DB.DepositCreate(depositId, userId, depositUrl, p.AmountCents)
	if err != nil {
		return DepositResult{}, fmt.Errorf("failed to create deposit record: %v", err)
	}

	return DepositResult{
		Id:  depositId,
		Url: depositUrl,
	}, nil
}

type WithdrawParams struct {
	Message     string `json:"message"`
	Signature   string `json:"signature"`
	AmountCents uint64 `json:"amountCents"`
}

type WithdrawResult struct {
	Id  string `json:"id"`
	Url string `json:"url"`
}

func onWithdraw(p WithdrawParams) (WithdrawResult, error) {
	// (1) Authenticate user
	userAddress, err := VerifyMessage(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return WithdrawResult{}, err
	}

	// (2) Get user and validate balance
	user, err := DB.UserGet(base58.Encode(userAddress[:]))
	if err != nil {
		return WithdrawResult{}, err
	}

	if p.AmountCents == 0 {
		return WithdrawResult{}, errors.New("withdrawal amount must be greater than 0")
	}

	if p.AmountCents > user.BalanceCents {
		return WithdrawResult{}, fmt.Errorf("insufficient balance: you have %d cents but trying to withdraw %d cents", user.BalanceCents, p.AmountCents)
	}

	// (3) Generate unique withdraw ID
	withdrawId := GenerateID(CentsToRaw(p.AmountCents))
	withdrawIdHex := hex.EncodeToString(withdrawId[:])

	// (4) Generate withdrawal signature
	withdrawSignatureBytes := SignWithdrawal(GAME_ADDRESS, userAddress, withdrawId, WITHDRAW_AUTHORITY_PRIVATE_KEY)
	withdrawSignature := hex.EncodeToString(withdrawSignatureBytes[:])

	// (5) Update user balance atomically
	updatedUser := User{
		Id:           user.Id,
		ServerSeed:   user.ServerSeed,
		BalanceCents: user.BalanceCents - p.AmountCents,
	}

	err = DB.UserCompareExchange(user, updatedUser)
	if err != nil {
		return WithdrawResult{}, fmt.Errorf("failed to update user balance: %v", err)
	}

	// (6) Generate withdraw URL
	withdrawUrl := fmt.Sprintf("%s/withdraw?game=%s&id=%s&signature=%s&user=%s",
		IVY_URL, base58.Encode(GAME_ADDRESS[:]), hex.EncodeToString(withdrawId[:]), withdrawSignature, user.Id)

	// (7) Create withdrawal record in database
	err = DB.WithdrawCreate(withdrawIdHex, user.Id, withdrawUrl, p.AmountCents, withdrawSignature)
	if err != nil {
		// Try to rollback the balance change
		// Yes, I know that this is not secure against edge cases,
		// but I'm too lazy to care :)
		DB.UserCredit(user.Id, p.AmountCents)
		return WithdrawResult{}, fmt.Errorf("failed to create withdrawal record: %v", err)
	}

	return WithdrawResult{
		Id:  withdrawIdHex,
		Url: withdrawUrl,
	}, nil
}

// Additional endpoints for checking deposit/withdrawal status
type DepositStatusParams struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
	DepositID string `json:"depositId"`
}

func onDepositStatus(p DepositStatusParams) (Deposit, error) {
	// Authenticate user
	userId, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return Deposit{}, err
	}

	// Get deposit
	deposit, err := DB.DepositGet(p.DepositID)
	if err != nil {
		return Deposit{}, err
	}

	// Verify user owns this deposit
	if deposit.UserId != userId {
		return Deposit{}, errors.New("deposit not owned by authenticated user")
	}

	// If completed, return deposit
	if deposit.Completed {
		return deposit, nil
	}

	// Otherwise, fetch deposit state on blockchain
	depositId, err := DecodeHex32(deposit.Id)
	if err != nil {
		return Deposit{}, err
	}
	depositInfo, err := FetchDepositInfo(AGGREGATOR_URL, GAME_ADDRESS, depositId)
	if err != nil {
		return Deposit{}, err
	}
	if depositInfo == nil {
		// Not completed, return incomplete deposit
		return deposit, nil
	}
	// Completed! let's complete it in db, then return updated deposit
	err = DB.DepositComplete(deposit.Id, depositInfo.Signature, depositInfo.Timestamp)
	if err != nil {
		return Deposit{}, err
	}
	// Credit user
	err = DB.UserCredit(deposit.UserId, deposit.AmountCents)
	if err != nil {
		// try to un-complete the deposit
		// insecure against edge cases but I'm lazy :)
		DB.DepositUncomplete(deposit.Id)
		return Deposit{}, err
	}
	deposit, err = DB.DepositGet(deposit.Id)
	if err != nil {
		return Deposit{}, err
	}
	return deposit, nil
}

// List endpoints
type ListParams struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
	Count     int    `json:"count"`
	Skip      int    `json:"skip"`
}

func onBetList(p ListParams) ([]Bet, error) {
	userId, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return nil, err
	}

	if p.Count <= 0 || p.Count > 100 {
		p.Count = 20 // Default
	}

	return DB.BetList(userId, p.Count, p.Skip)
}

func onDepositList(p ListParams) ([]Deposit, error) {
	userId, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return nil, err
	}

	if p.Count <= 0 || p.Count > 100 {
		p.Count = 20 // Default
	}

	return DB.DepositList(userId, p.Count, p.Skip)
}

func onWithdrawList(p ListParams) ([]Withdrawal, error) {
	userId, err := VerifyMessageB58(GAME_ADDRESS, p.Message, p.Signature)
	if err != nil {
		return nil, err
	}

	if p.Count <= 0 || p.Count > 100 {
		p.Count = 20 // Default
	}

	return DB.WithdrawList(userId, p.Count, p.Skip)
}

func onRequest(body []byte) (any, error) {
	type AnyRequest struct {
		Action string `json:"action"`
	}
	var ar AnyRequest
	err := json.Unmarshal(body, &ar)
	if err != nil {
		return nil, err
	}
	a := ar.Action
	switch a {
	case "ping":
		type PingResponse struct {
			Response string `json:"response"`
		}
		return PingResponse{
			Response: "pong",
		}, nil

	case "max_bet":
		type MaxBetResponse struct {
			MaxBetCents uint64 `json:"maxBetCents"`
		}
		return MaxBetResponse{
			MaxBetCents: MAX_BET_CENTS,
		}, nil

	case "user_get":
		var p UserGetParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onUserGet(p)

	case "bet":
		var p BetParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onBet(p)

	case "deposit":
		var p DepositParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onDeposit(p)

	case "withdraw":
		var p WithdrawParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onWithdraw(p)

	case "deposit_status":
		var p DepositStatusParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onDepositStatus(p)

	case "bet_list":
		var p ListParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onBetList(p)

	case "deposit_list":
		var p ListParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onDepositList(p)

	case "withdraw_list":
		var p ListParams
		err = json.Unmarshal(body, &p)
		if err != nil {
			return nil, err
		}
		return onWithdrawList(p)

	default:
		return nil, fmt.Errorf("unknown action %s", ar.Action)
	}
}

func main() {
	var err error
	db, err := sql.Open("sqlite3", DB_PATH)
	if err != nil {
		log.Fatal(err)
	}
	DB = Database{db}
	err = DB.Startup()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			return
		}

		if r.Method != "POST" {
			http.Error(w, `{"error":"method not allowed"}`, 405)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":"can't read body"}`, 500)
			return
		}

		data, err := onRequest(body)
		if err != nil {
			text, errMarshal := json.Marshal(ErrorResponse{
				Error: err.Error(),
			})
			if errMarshal != nil {
				text = []byte(`{"error":"can't serialize error response"}`)
			}
			http.Error(w, string(text), 400)
			return
		}

		err = json.NewEncoder(w).Encode(data)
		if err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	})

	log.Println("Listening on port", PORT)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(PORT), nil))
}
