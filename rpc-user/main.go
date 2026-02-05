package main

import (
	"log"
	user "rpc-user/kitex_gen/user/userservice"
)

func main() {
	svr := user.NewServer(new(UserServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
