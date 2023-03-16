package store

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/metal-toolbox/alloy/internal/app"
	"github.com/metal-toolbox/alloy/internal/model"
	"github.com/stretchr/testify/assert"
)

// This type exists to allow the csv bytes to be passed around as a io.ReadCloser
// which is expected by the csv asset loader
type ioReaderFakeCloser struct {
	io.Reader
}

func (i ioReaderFakeCloser) Close() error {
	return nil
}

func Test_RunCSVListAll(t *testing.T) {
	// init alloy app
	alloy, err := app.New(context.TODO(), model.AppKindOutOfBand, "", model.LogLevelInfo)
	if err != nil {
		t.Fatal(err)
	}

	got := []*model.Asset{}
	expected := []*model.Asset{
		{
			ID:          "070db820-d807-013a-c0bf-3e22fbc86c7a",
			BMCUsername: "foo",
			BMCPassword: "bar",
			BMCAddress:  net.ParseIP("127.0.0.1"),
		},
		{
			ID:          "050db820-c807-013a-c0bf-3e22fbc86c5a",
			BMCUsername: "admin",
			BMCPassword: "hunter2",
			BMCAddress:  net.ParseIP("127.0.0.2"),
		},
	}

	var collectTimedOut bool

	// background routine to receive asset objects objects sent from the asset getter
	// mocks a collector
	alloy.SyncWg.Add(1)

	go func(t *testing.T, wg *sync.WaitGroup) {
		t.Helper()

		defer wg.Done()

		timeout := time.NewTicker(time.Second * 5).C
	Loop:
		for {
			select {
			case asset, ok := <-alloy.AssetCh:
				if !ok {
					break Loop
				}
				got = append(got, asset)
			case <-timeout:
				collectTimedOut = true
				break Loop
			}
		}
	}(t, alloy.SyncWg)

	csvee := []byte(`id,ipaddress,username,password
070db820-d807-013a-c0bf-3e22fbc86c7a,127.0.0.1,foo,bar
050db820-c807-013a-c0bf-3e22fbc86c5a,127.0.0.2,admin,hunter2`)

	// init asset getter
	csvs, err := NewCSVGetter(context.TODO(), alloy, ioReaderFakeCloser{bytes.NewReader(csvee)})
	if err != nil {
		t.Fatal(err)
	}

	err = csvs.ListAll(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	// wait for routines to complete
	alloy.SyncWg.Wait()

	// test inventory items match expected
	assert.ElementsMatch(t, expected, got)
	assert.False(t, collectTimedOut)
}
