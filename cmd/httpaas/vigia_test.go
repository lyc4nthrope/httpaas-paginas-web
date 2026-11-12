package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestVigilarReportsAndClearsByItself(t *testing.T) {
	var caida atomic.Bool
	caida.Store(true)
	check := func(context.Context) error {
		if caida.Load() {
			return errors.New("no se contacta la VM DNS en 192.168.10.10")
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	aviso := vigilar(ctx, check, 5*time.Millisecond)
	if aviso() == "" {
		t.Fatal("the first check runs before returning, so a down ns is reported at once")
	}
	caida.Store(false) // ns is started after the app
	limite := time.Now().Add(2 * time.Second)
	for aviso() != "" {
		if time.Now().After(limite) {
			t.Fatal("the warning must clear by itself once ns answers, without a restart")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestVigilarStopsWithTheContext(t *testing.T) {
	var llamadas atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	vigilar(ctx, func(context.Context) error { llamadas.Add(1); return nil }, time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	antes := llamadas.Load()
	time.Sleep(20 * time.Millisecond)
	if llamadas.Load() != antes {
		t.Fatal("checks must stop when the app shuts down")
	}
}
