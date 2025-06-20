package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Replace the placeholder with your Atlas connection string
const logsUriWithADF = "mongodb://atlas-online-archive-65562108b627326cf1af4035-fisqe.g.query.mongodb.net/?ssl=true&authSource=admin"
const logsUriWithADFSpecial = "mongodb://archived-atlas-online-archive-65562108b627326cf1af4035-fisqe.g.query.mongodb.net/?directConnection=true&tls=true&authSource=admin"
const logsUriNoADF = "mongodb+srv://logs.fisqe.mongodb.net/"

// const logsUri = logsUriNoADF
const logsUri = logsUriWithADFSpecial

const mainUri = "mongodb+srv://main.fisqe.mongodb.net/"

// Just MDB: mongodb+srv://logs.fisqe.mongodb.net/

type Arg struct {
	ctx           context.Context
	AtlasUsername string
	AtlasPassword string
}

func pullLogs(ctx context.Context, client *mongo.Client, partId string, start, end time.Time) *mongo.Cursor {
	query := bson.M{"robot_part_id": partId}
	timeFilter := bson.M{}
	if !start.IsZero() {
		timeFilter["$gte"] = start
	}
	if !end.IsZero() {
		timeFilter["$lte"] = end
	}
	if !start.IsZero() || !end.IsZero() {
		query["log.time"] = timeFilter
	}
	cursor, err := client.Database("main").Collection("robot_log").Find(ctx, query, options.Find().SetSort(bson.M{"log.time": 1}))
	if err != nil {
		panic(err)
	}

	return cursor
}

func searchForPartByName(ctx context.Context, client *mongo.Client, partName string) []RobotPart {
	var ret []RobotPart
	cursor, err := client.Database("main").Collection("robot_part").Find(ctx, bson.M{
		"name": bson.M{
			"$in": []string{partName, fmt.Sprintf("%v-main", partName)},
		},
	})
	if err != nil {
		panic(err)
	}
	err = cursor.All(ctx, &ret)
	if err != nil {
		panic(err)
	}

	for idx := range ret {
		if cfgAsJSONBytes, err := json.Marshal(ret[idx].Config); err == nil {
			ret[idx].ConfigAsJSON = string(cfgAsJSONBytes)
		} else {
			log.Println("Error marshaling as json:", err)
			ret[idx].ConfigAsJSON = fmt.Sprintf("%v", ret[idx].Config)
		}
	}

	return ret
}
