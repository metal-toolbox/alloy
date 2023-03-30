package helpers

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/metal-toolbox/alloy/internal/model"

	// nolint:gosec // pprof path is only exposed over localhost
	_ "net/http/pprof"
)

// EnablePProfile enables the profiling endpoint
func EnablePProfile() {
	go func() {
		server := &http.Server{
			Addr:              model.ProfilingEndpoint,
			ReadHeaderTimeout: 2 * time.Second, // nolint:gomnd // time duration value is clear as is.
		}

		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	log.Println("profiling enabled: " + model.ProfilingEndpoint + "/debug/pprof")
}

func MapsAreEqual(currentMap, newMap map[string]string) bool {
	if len(currentMap) != len(newMap) {
		return false
	}

	for k, currVal := range currentMap {
		newVal, keyExists := newMap[k]
		if !keyExists {
			return false
		}

		if newVal != currVal {
			return false
		}
	}

	return true
}

func WriteDebugFile(name, dump string) {
	// nolint:gomnd // file permission is clear as is
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	defer f.Close()

	_, _ = f.WriteString(dump)
}
