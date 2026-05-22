package olddb

import "github.com/jackc/pgx/v5/pgxpool"

// DB is a connection to the legacy monolithic chat service database.
type DB struct {
	pool *pgxpool.Pool

	botStore          *BotStore
	clientStore       *ClientStore
	conversationStore *ConversationStore
	messageStore      *MessageStore
}

func New(pool *pgxpool.Pool) *DB { return &DB{pool: pool} }

func (db *DB) Pool() *pgxpool.Pool { return db.pool }

func (db *DB) Close() { db.pool.Close() }

func (db *DB) BotStore() *BotStore {
	if db.botStore == nil {
		db.botStore = NewBotStore(db)
	}
	return db.botStore
}

func (db *DB) ClientStore() *ClientStore {
	if db.clientStore == nil {
		db.clientStore = NewClientStore(db)
	}
	return db.clientStore
}

func (db *DB) ConversationStore() *ConversationStore {
	if db.conversationStore == nil {
		db.conversationStore = NewConversationStore(db)
	}
	return db.conversationStore
}

func (db *DB) MessageStore() *MessageStore {
	if db.messageStore == nil {
		db.messageStore = NewMessageStore(db)
	}
	return db.messageStore
}
