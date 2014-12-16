package auctioneer

import "github.com/tedsuo/rata"

const (
	CreateTaskAuctionRoute = "CreateTaskAuction"
	CreateLRPAuctionRoute  = "CreateLRPAuction"
)

var Routes = rata.Routes{
	{Path: "/tasks", Method: "POST", Name: CreateTaskAuctionRoute},
	{Path: "/lrps", Method: "POST", Name: CreateLRPAuctionRoute},
}
