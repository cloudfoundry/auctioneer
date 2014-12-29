package auctioneer

import "github.com/tedsuo/rata"

const (
	CreateTaskAuctionsRoute = "CreateTaskAuctions"
	CreateLRPAuctionsRoute  = "CreateLRPAuctions"
)

var Routes = rata.Routes{
	{Path: "/tasks", Method: "POST", Name: CreateTaskAuctionsRoute},
	{Path: "/lrps", Method: "POST", Name: CreateLRPAuctionsRoute},
}
