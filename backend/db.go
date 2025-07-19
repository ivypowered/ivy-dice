package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
)

type User struct {
	Id           string `json:"id"`
	ServerSeed   string `json:"serverSeed"`
	BalanceCents uint64 `json:"balanceCents"`
}

type Deposit struct {
	Id          string  `json:"id"`
	UserId      string  `json:"userId"`
	Url         string  `json:"url"`
	AmountCents uint64  `json:"amountCents"`
	Completed   bool    `json:"completed"`
	Signature   string  `json:"signature"`
	CreatedAt   uint64  `json:"createdAt"`
	CompletedAt *uint64 `json:"completedAt,omitempty"`
}

type Withdrawal struct {
	Id          string `json:"id"`
	UserId      string `json:"userId"`
	Url         string `json:"url"`
	AmountCents uint64 `json:"amountCents"`
	Signature   string `json:"signature"`
	CreatedAt   uint64 `json:"createdAt"`
}

type Bet struct {
	Id          uint64 `json:"id"`
	UserId      string `json:"userId"`
	AmountCents uint64 `json:"amountCents"`
	RollUnder   bool   `json:"rollUnder"`
	Threshold   uint16 `json:"threshold"`
	Result      uint16 `json:"result"`
	Won         bool   `json:"won"`
	ServerSeed  string `json:"serverSeed"`
	CreatedAt   uint64 `json:"createdAt"`
}

type Database struct {
	*sql.DB
}

func (db Database) Startup() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        serverSeed TEXT NOT NULL,
        balanceCents INTEGER NOT NULL
    )`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS bets (
        id INTEGER PRIMARY KEY,
        userId TEXT NOT NULL,
        amountCents INTEGER NOT NULL,
        rollUnder BOOLEAN NOT NULL,
        threshold INTEGER NOT NULL,
        result INTEGER NOT NULL,
        won BOOLEAN NOT NULL,
        serverSeed TEXT NOT NULL,
        createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
        FOREIGN KEY (userId) REFERENCES users(id)
    )`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxBetsUserId ON bets(userId)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxBetsCreatedAt ON bets(createdAt)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS deposits (
        id TEXT PRIMARY KEY,
        userId TEXT NOT NULL,
        url TEXT NOT NULL,
        amountCents INTEGER NOT NULL,
        completed BOOLEAN NOT NULL DEFAULT 0,
        signature TEXT,
        createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
        completedAt INTEGER,
        FOREIGN KEY (userId) REFERENCES users(id)
    )`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxDepositsUserId ON deposits(userId)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxDepositsCreatedAt ON deposits(createdAt)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS withdrawals (
        id TEXT PRIMARY KEY,
        userId TEXT NOT NULL,
        url TEXT NOT NULL,
        amountCents INTEGER NOT NULL,
        signature TEXT NOT NULL,
        createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
        FOREIGN KEY (userId) REFERENCES users(id)
    )`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxWithdrawalsUserId ON withdrawals(userId)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idxWithdrawalsCreatedAt ON withdrawals(createdAt)`)
	if err != nil {
		return err
	}

	return nil
}

func (db Database) UserGet(id string) (User, error) {
	var serverSeed string
	var balanceCents uint64
	err := db.QueryRow("SELECT serverSeed, balanceCents FROM users WHERE id = ?", id).Scan(&serverSeed, &balanceCents)
	if err == sql.ErrNoRows {
		seedBytes := NewServerSeed()
		serverSeed = hex.EncodeToString(seedBytes[:])
		balanceCents = 0
		_, err = db.Exec(`INSERT INTO users (id, serverSeed, balanceCents) VALUES (?, ?, ?)`, id, serverSeed, balanceCents)
	}
	if err != nil {
		return User{}, err
	}
	return User{
		Id:           id,
		ServerSeed:   serverSeed,
		BalanceCents: balanceCents,
	}, nil
}

func (db Database) UserCredit(id string, amountCents uint64) error {
	res, err := db.Exec("UPDATE users SET balanceCents = balanceCents + ? WHERE id = ?", amountCents, id)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("could not credit user: no rows affected")
	}
	return nil
}

func (db Database) UserCompareExchange(expected User, desired User) error {
	if expected.Id != desired.Id {
		return errors.New("mismatching user IDs for compare-and-swap")
	}
	result, err := db.Exec(
		`UPDATE users SET serverSeed = ?, balanceCents = ? WHERE id = ? AND serverSeed = ? AND balanceCents = ?`,
		desired.ServerSeed, desired.BalanceCents, expected.Id, expected.ServerSeed, expected.BalanceCents,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected < 1 {
		return errors.New("user compare-and-swap failed: no matching row found")
	}
	return nil
}

func (db Database) DepositCreate(id string, userId string, url string, amountCents uint64) error {
	_, err := db.Exec(`INSERT INTO deposits (id, userId, url, amountCents) VALUES (?, ?, ?, ?)`,
		id, userId, url, amountCents)
	return err
}

func (db Database) DepositGet(id string) (Deposit, error) {
	var deposit Deposit
	var signature sql.NullString
	var completedAt sql.NullInt64

	err := db.QueryRow(`SELECT id, userId, url, amountCents, completed, signature, createdAt, completedAt
                        FROM deposits WHERE id = ?`, id).Scan(
		&deposit.Id, &deposit.UserId, &deposit.Url, &deposit.AmountCents, &deposit.Completed,
		&signature, &deposit.CreatedAt, &completedAt)

	if signature.Valid {
		deposit.Signature = signature.String
	}
	if completedAt.Valid {
		completedAt := uint64(completedAt.Int64)
		deposit.CompletedAt = &completedAt
	}

	return deposit, err
}

func (db Database) DepositUncomplete(id string) error {
	result, err := db.Exec(`UPDATE deposits SET completed = 0 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	aff, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if aff < 1 {
		return errors.New("could not uncomplete deposit: can't find provided ID in db")
	}
	return nil
}

func (db Database) DepositComplete(id string, signature string, timestamp uint64) error {
	result, err := db.Exec(`UPDATE deposits SET completed = 1, signature = ?, completedAt = ?
                            WHERE id = ? AND completed = 0`, signature, timestamp, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected < 1 {
		return errors.New("deposit not found or already completed")
	}
	return nil
}

func (db Database) DepositList(userId string, count int, skip int) ([]Deposit, error) {
	rows, err := db.Query(`SELECT id, userId, url, amountCents, completed, signature, createdAt, completedAt
                          FROM deposits WHERE userId = ? ORDER BY createdAt DESC LIMIT ? OFFSET ?`,
		userId, count, skip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deposits []Deposit
	for rows.Next() {
		var deposit Deposit
		var signature sql.NullString
		var completedAt sql.NullInt64

		err := rows.Scan(&deposit.Id, &deposit.UserId, &deposit.Url, &deposit.AmountCents, &deposit.Completed,
			&signature, &deposit.CreatedAt, &completedAt)
		if err != nil {
			return nil, err
		}

		if signature.Valid {
			deposit.Signature = signature.String
		}
		if completedAt.Valid {
			completedAt := uint64(completedAt.Int64)
			deposit.CompletedAt = &completedAt
		}

		deposits = append(deposits, deposit)
	}

	return deposits, nil
}

func (db Database) WithdrawCreate(id string, userId string, url string, amountCents uint64, signature string) error {
	_, err := db.Exec(`INSERT INTO withdrawals (id, userId, url, amountCents, signature) VALUES (?, ?, ?, ?, ?)`,
		id, userId, url, amountCents, signature)
	return err
}

func (db Database) WithdrawGet(id string) (Withdrawal, error) {
	var withdrawal Withdrawal

	err := db.QueryRow(`SELECT id, userId, url, amountCents, signature, createdAt
                        FROM withdrawals WHERE id = ?`, id).Scan(
		&withdrawal.Id, &withdrawal.UserId, &withdrawal.Url, &withdrawal.AmountCents,
		&withdrawal.Signature, &withdrawal.CreatedAt)

	return withdrawal, err
}

func (db Database) WithdrawList(userId string, limit int, offset int) ([]Withdrawal, error) {
	rows, err := db.Query(`SELECT id, userId, url, amountCents, signature, createdAt
                          FROM withdrawals WHERE userId = ? ORDER BY createdAt DESC LIMIT ? OFFSET ?`,
		userId, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []Withdrawal
	for rows.Next() {
		var withdrawal Withdrawal
		err := rows.Scan(&withdrawal.Id, &withdrawal.UserId, &withdrawal.Url, &withdrawal.AmountCents,
			&withdrawal.Signature, &withdrawal.CreatedAt)
		if err != nil {
			return nil, err
		}

		withdrawals = append(withdrawals, withdrawal)
	}

	return withdrawals, nil
}

func (db Database) BetCreate(b Bet) error {
	_, err := db.Exec(`INSERT INTO bets (userId, amountCents, rollUnder, threshold, result, won, serverSeed)
                       VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.UserId, b.AmountCents, b.RollUnder, b.Threshold, b.Result, b.Won, b.ServerSeed)
	return err
}

func (db Database) BetList(userId string, count int, skip int) ([]Bet, error) {
	rows, err := db.Query(`SELECT id, userId, amountCents, rollUnder, threshold, result, won, serverSeed, createdAt
                          FROM bets WHERE userId = ? ORDER BY createdAt DESC LIMIT ? OFFSET ?`,
		userId, count, skip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bets []Bet
	for rows.Next() {
		var bet Bet
		err := rows.Scan(&bet.Id, &bet.UserId, &bet.AmountCents, &bet.RollUnder,
			&bet.Threshold, &bet.Result, &bet.Won, &bet.ServerSeed, &bet.CreatedAt)
		if err != nil {
			return nil, err
		}
		bets = append(bets, bet)
	}

	return bets, nil
}
