package db

type Database interface {
	Get(key interface{}) (interface{}, error)
	Put(key interface{}, value interface{}) error
	Delete(key interface{}) error
	Close()
}
