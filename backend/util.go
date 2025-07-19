package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/mr-tron/base58"
)

func MustDecodeBase58PublicKey(k string) [32]byte {
	bytes, err := base58.Decode(k)
	if err != nil {
		panic("can't decode base58 key: " + err.Error())
	}
	if len(bytes) != 32 {
		panic("can't decode base58 key: required length 32, got " + strconv.Itoa(len(bytes)))
	}
	var b [32]byte
	copy(b[:], bytes[:])
	return b
}

func MustDecodeHexPrivateKey(k string) [64]byte {
	bytes, err := hex.DecodeString(k)
	if err != nil {
		panic("can't decode hex key: " + err.Error())
	}
	if len(bytes) != 64 {
		panic("can't decode hex key: required length 64, got " + strconv.Itoa(len(bytes)))
	}
	var b [64]byte
	copy(b[:], bytes[:])
	return b
}

// Sign a withdrawal message using ed25519
func SignWithdrawal(game [32]byte, user [32]byte, id [32]byte, privkey [64]byte) [64]byte {
	privateKey := ed25519.PrivateKey(privkey[:])

	// Create the message: game_address (32 bytes) + user_key (32 bytes) + withdraw_id (32 bytes)
	message := make([]byte, 0, 96)
	message = append(message, game[:]...)
	message = append(message, user[:]...)
	message = append(message, id[:]...)

	// Sign the message
	signature := ed25519.Sign(privateKey, message)
	var s [64]byte
	copy(s[:], signature[:])
	return s
}

var VERIFY_MESSAGE_REGEX = regexp.MustCompile(
	`^Authenticate user ([1-9A-Za-z]+) to game ([1-9A-Za-z]+) on ivypowered\.com, valid from ([0-9]+) to ([0-9]+)$`,
)

// Verify an authentication message and return the authenticated user, or an error
// if the message is invalid.
func VerifyMessage(game [32]byte, message string, signature string) ([32]byte, error) {
	matches := VERIFY_MESSAGE_REGEX.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 || len(matches[0]) < 5 {
		return [32]byte{}, errors.New("invalid auth message format")
	}
	userBytes, err := base58.Decode(matches[0][1])
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid user format in auth message: %v", err)
	}
	if len(userBytes) != ed25519.PublicKeySize {
		return [32]byte{}, errors.New("user public key is too large in auth message")
	}
	user := ed25519.PublicKey(userBytes)
	gameBytes, err := base58.Decode(matches[0][2])
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid game format in auth message: %v", err)
	}
	if len(gameBytes) != ed25519.PublicKeySize {
		return [32]byte{}, errors.New("game public key is too large in auth message")
	}
	gameProvided := ed25519.PublicKey(gameBytes)
	if !gameProvided.Equal(ed25519.PublicKey(game[:])) {
		return [32]byte{}, fmt.Errorf("invalid game in auth message (provided %s, expected %s)", base58.Encode(gameProvided), base58.Encode(game[:]))
	}
	from, err := strconv.ParseUint(matches[0][3], 10, 0)
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid from format in auth message: %v", err)
	}
	to, err := strconv.ParseUint(matches[0][4], 10, 0)
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid to format in auth message: %v", err)
	}
	now := uint64(time.Now().Unix())
	if now < from || now > to {
		return [32]byte{}, fmt.Errorf("expiry of auth message: only valid from %d to %d, but time is %d", from, to, now)
	}
	sig := make([]byte, 64)
	n, err := hex.Decode(sig, []byte(signature))
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid signature in auth message: %v", err)
	}
	if n != 64 {
		return [32]byte{}, fmt.Errorf("signature invalid length in auth message: expected 64, got %d", n)
	}
	valid := ed25519.Verify(user, []byte(message), sig)
	if !valid {
		return [32]byte{}, errors.New("invalid auth message signature")
	}
	u := [32]byte{}
	copy(u[:], user[:])
	return u, nil
}

// Just like VerifyMessage, but returns the user as a base58-encoded string.
func VerifyMessageB58(game [32]byte, message string, signature string) (string, error) {
	bytes, err := VerifyMessage(game, message, signature)
	if err != nil {
		return "", err
	}
	return base58.Encode(bytes[:]), nil
}

// Convert cents to raw
// 1 unit = 10^9 raw
// 1 cent = 0.01 units = 10^7 raw
func CentsToRaw(cents uint64) uint64 {
	return cents * 10_000_000
}

// Generate a 32-byte unique deposit/withdraw ID
func GenerateID(amountRaw uint64) [32]byte {
	var id [32]byte
	_, err := io.ReadFull(rand.Reader, id[:24])
	if err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(id[24:], amountRaw)
	return id
}

// Decode a hex string into a 32-byte array
func DecodeHex32(s string) ([32]byte, error) {
	bytes, err := hex.DecodeString(s)
	if err != nil {
		return [32]byte{}, err
	}
	if len(bytes) != 32 {
		return [32]byte{}, errors.New("length of bytes is not 32!")
	}
	var b [32]byte
	copy(b[:], bytes[:])
	return b, nil
}

type DepositInfo struct {
	Signature string `json:"signature"`
	Timestamp uint64 `json:"timestamp"`
}

type DepositSuccess struct {
	Status string       `json:"status"`
	Data   *DepositInfo `json:"data"`
}

type DepositError struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

// Fetches the deposit info. `*DepositInfo` will be nil on success if no deposit exists
func FetchDepositInfo(aggregator_url string, game [32]byte, id [32]byte) (*DepositInfo, error) {
	url := AGGREGATOR_URL + "/games/" + base58.Encode(game[:]) + "/deposits/" + hex.EncodeToString(id[:])
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var de DepositError
	err = json.Unmarshal(bytes, &de)
	if err == nil && de.Status == "err" {
		return nil, errors.New(de.Msg)
	}
	var ds DepositSuccess
	err = json.Unmarshal(bytes, &ds)
	if err != nil {
		return nil, err
	}
	return ds.Data, nil
}
