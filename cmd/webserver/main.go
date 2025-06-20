package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/viamrobotics/bfserver/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Arg struct {
	ctx           context.Context
	AtlasUsername string
	AtlasPassword string
}

func ParseProgramArgs() *Arg {
	var ret Arg
	// In args because timeouts may be passed from CLI?
	ret.ctx = context.Background()

	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}

	// Why not share the bf server secrets file? Maybe just have a "global" .config/secrets file.
	secretsFile, err := os.Open(fmt.Sprintf("%v/bfserver/secrets", configDir))
	if err != nil {
		panic(err)
	}
	defer secretsFile.Close()

	scanner := bufio.NewScanner(secretsFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			fmt.Println("Bad secrets line:", scanner.Text())
			continue
		}

		switch key {
		case "atlas_logs_username":
			ret.AtlasUsername = value
		case "atlas_logs_password":
			ret.AtlasPassword = value
		}
	}

	return &ret
}

const mainUri = "mongodb+srv://main.fisqe.mongodb.net/"
const logsUriNoADF = "mongodb+srv://logs.fisqe.mongodb.net/"

func (args *Arg) createMDBClient(ctx context.Context) *mongo.Client {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mainUri).
		SetAuth(options.Credential{Username: args.AtlasUsername, Password: args.AtlasPassword}))
	if err != nil {
		panic(err)
	}

	return client
}

func main() {
	ctx := context.Background()
	args := ParseProgramArgs()
	mux := service.NewWebServer(ctx, args.createMDBClient(ctx))

	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		panic(err)
	}
}
