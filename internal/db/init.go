package db

import (
	"log"
)

func RegisterDB() error {
	err := RedisInit()
	if err != nil {
		return err
	}
	log.Println("Redis init done")
	if err = InitMysql(); err != nil {
		return err
	}
	log.Println("Mysql init done")
	// if err = InitMongoClient(); err != nil {
	// 	return err
	// }
	//log.Println("Mongo init done")
	log.Println("Db init done")
	return nil
}
