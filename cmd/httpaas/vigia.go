package main

import (
	"context"
	"sync/atomic"
	"time"
)

// vigilar runs check now and then every cada until ctx ends, and returns a function that
// gives the latest result: "" when check passed, otherwise its error message. Checking
// again in the background means the dashboard warning clears by itself when the ns VM
// is started after the app, with no restart (the lesson of the Parcial 2 console).
func vigilar(ctx context.Context, check func(context.Context) error, cada time.Duration) func() string {
	var ultimo atomic.Value
	revisar := func() {
		msg := ""
		if err := check(ctx); err != nil {
			msg = err.Error()
		}
		ultimo.Store(msg)
	}
	revisar()
	go func() {
		t := time.NewTicker(cada)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				revisar()
			}
		}
	}()
	return func() string { return ultimo.Load().(string) }
}
