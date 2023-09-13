package main

import "github.com/viamrobotics/bfserver/service"

func main() {
	server := service.NewBFServer()
	server.Start()
}
