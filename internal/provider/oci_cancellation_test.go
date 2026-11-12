package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
)

func TestOCIInFlightCancellationStopsTransportWithoutSDKRace(t *testing.T) {
	for i := 0; i < 25; i++ {
		started := make(chan struct{})
		stopped := make(chan struct{})
		p := testComputeProvider(t, func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
			close(stopped)
		})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := p.WaitInstance(ctx, "instance", domain.StatusRunning)
			done <- err
		}()
		select {
		case <-started:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("HTTP request did not start")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("caller cancellation was lost: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("SDK call did not finish after cancellation")
		}
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("HTTP transport was not cancelled")
		}
	}
}
