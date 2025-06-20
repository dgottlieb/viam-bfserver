package service

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type RobotPart struct {
	Id               string `bson:"_id"`
	Name             string `bson:"name"`
	Config           bson.M `bson:"config"`
	ConfigAsJSON     string
	UserSuppliedInfo struct {
		IPs []string `bson:"ips"`
	} `bson:"user_supplied_info"`
	MainPart   bool      `bson:"main_part"`
	DnsName    string    `bson:"dns_name"`
	LocationId string    `bson:"location_id"`
	LastOnline time.Time `bson:"last_online"`
}
