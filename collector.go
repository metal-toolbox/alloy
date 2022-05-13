package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/metal-toolbox/ironlib"
	"github.com/sirupsen/logrus"
)

// collector collects hardware inventory
type collector struct {
	component string
	serverURL string
	localFile string
	verbose   bool
	dryRun    bool
}

func (*collector) inventory() error {
	logger := logrus.New()

	device, err := ironlib.New(logger)
	if err != nil {
		logger.Fatal(err)
	}

	inv, err := device.GetInventory(context.TODO())
	if err != nil {
		logger.Fatal(err)
	}

	j, err := json.MarshalIndent(inv, " ", "  ")
	if err != nil {
		logger.Fatal(err)
	}

	fmt.Println(j)

	return nil
}
