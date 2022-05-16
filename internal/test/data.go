package test

import "os"

type Data struct {
	Rd
	subscriptionId string
}

func NewData() Data {
	return Data{
		Rd:             NewRd(),
		subscriptionId: os.Getenv("ARM_SUBSCRIPTION_ID"),
	}
}
