package test

import "os"

type Data struct {
	Rd
	SubscriptionId string
}

func NewData() Data {
	return Data{
		Rd:             NewRd(),
		SubscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
	}
}
