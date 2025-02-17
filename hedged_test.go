package hedgedhttp_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cristalhq/hedgedhttp"
)

func TestValidateInput(t *testing.T) {
	_, _, err := hedgedhttp.NewClient(-time.Second, 0, nil)
	if err == nil {
		t.Fatalf("want err, got nil")
	}

	_, _, err = hedgedhttp.NewClient(time.Second, -1, nil)
	if err == nil {
		t.Fatalf("want err, got nil")
	}

	_, _, err = hedgedhttp.NewClient(time.Second, 0, nil)
	if err == nil {
		t.Fatalf("want err, got nil")
	}

	_, _, err = hedgedhttp.NewRoundTripper(time.Second, 0, nil)
	if err == nil {
		t.Fatalf("want err, got nil")
	}
}

func TestUpto(t *testing.T) {
	var gotRequests int64

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&gotRequests, 1)
		time.Sleep(100 * time.Millisecond)
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	const upto = 7
	client, _, err := hedgedhttp.NewClient(10*time.Millisecond, upto, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotRequests := atomic.LoadInt64(&gotRequests); gotRequests != upto {
		t.Fatalf("want %v, got %v", upto, gotRequests)
	}
}

func TestUptoWithInstrumentation(t *testing.T) {
	var gotRequests int64

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&gotRequests, 1)
		time.Sleep(100 * time.Millisecond)
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	const upto = 7
	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, upto, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotRequests := atomic.LoadInt64(&gotRequests); gotRequests != upto {
		t.Fatalf("want %v, got %v", upto, gotRequests)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != upto {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != 0 {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > upto {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestNoTimeout(t *testing.T) {
	const sleep = 10 * time.Millisecond
	var gotRequests int64

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&gotRequests, 1)
		time.Sleep(sleep)
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	const upto = 10

	client, metrics, err := hedgedhttp.NewClient(0, upto, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotRequests := atomic.LoadInt64(&gotRequests); gotRequests < 1 || gotRequests > upto {
		t.Fatalf("want %v, got %v", upto, gotRequests)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips < 2 || actualRoundTrips > upto {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != 0 {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > upto {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestFirstIsOK(t *testing.T) {
	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, 10, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("want ok, got %s", string(body))
	}
	expectExactMetricsAndSnapshot(t, metrics, hedgedhttp.StatsSnapshot{
		RequestedRoundTrips:      1,
		ActualRoundTrips:         1,
		FailedRoundTrips:         0,
		CanceledByUserRoundTrips: 0,
		CanceledSubRequests:      0,
	})
}

func TestBestResponse(t *testing.T) {
	const shortest = 20 * time.Millisecond
	timeouts := [...]time.Duration{30 * shortest, 5 * shortest, shortest, shortest, shortest}
	timeoutCh := make(chan time.Duration, len(timeouts))
	for _, t := range timeouts {
		timeoutCh <- t
	}

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(<-timeoutCh)
	})

	start := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, 5, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	passed := time.Since(start)

	if float64(passed) > float64(shortest)*2.5 {
		t.Fatalf("want %v, got %v", shortest, passed)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips < 4 || actualRoundTrips > 5 {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != 0 {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > 4 {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestGetSuccessEvenWithErrorsPresent(t *testing.T) {
	var gotRequests uint64

	upto := uint64(5)
	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddUint64(&gotRequests, 1)
		if idx == upto {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("success")); err != nil {
				t.Fatal(err)
			}
			return
		}

		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		conn.Close() // emulate error by closing connection on client side
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, int(upto), nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected resp status code: %+v", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(respBytes, []byte("success")) {
		t.Fatalf("Unexpected resp body %+v; as string: %+v", respBytes, string(respBytes))
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != upto {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != upto-1 {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > 4 {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestGetFailureAfterAllRetries(t *testing.T) {
	const upto = 5

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		conn.Close() // emulate error by closing connection on client side
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	client, metrics, err := hedgedhttp.NewClient(time.Millisecond, upto, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err == nil {
		t.Fatal(err)
	}
	if resp != nil {
		t.Fatalf("Unexpected response %+v", resp)
	}

	wantErrStr := fmt.Sprintf(`%d errors occurred:`, upto)
	if !strings.Contains(err.Error(), wantErrStr) {
		t.Fatalf("Unexpected err %+v", err)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != upto {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != upto {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > upto {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestHangAllExceptLast(t *testing.T) {
	const upto = 5
	var gotRequests uint64
	blockCh := make(chan struct{})
	defer close(blockCh)

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.AddUint64(&gotRequests, 1)
		if idx == upto {
			time.Sleep(100 * time.Millisecond)
			return
		}
		<-blockCh
	})

	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, upto, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected resp status code: %+v", resp.StatusCode)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != upto {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != 0 {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 0 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > upto-1 {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestCancelByClient(t *testing.T) {
	blockCh := make(chan struct{})
	defer close(blockCh)

	url := testServerURL(t, func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	upto := 5
	client, metrics, err := hedgedhttp.NewClient(10*time.Millisecond, upto, nil)
	if err != nil {
		t.Fatalf("want nil, got %s", err)
	}

	checkAllMetricsAreZero(t, metrics)
	resp, err := client.Do(req)
	if err == nil {
		t.Fatal(err)
	}

	if resp != nil {
		t.Fatalf("Unexpected resp: %+v", resp)
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != 1 {
		t.Fatalf("Unnexpected requestedRoundTrips: %v", requestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != uint64(upto) {
		t.Fatalf("Unnexpected actualRoundTrips: %v", actualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips > uint64(upto) {
		t.Fatalf("Unnexpected failedRoundTrips: %v", failedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != 1 {
		t.Fatalf("Unnexpected canceledByUserRoundTrips: %v", canceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests > uint64(upto) {
		t.Fatalf("Unnexpected canceledSubRequests: %v", canceledSubRequests)
	}
}

func TestIsHedged(t *testing.T) {
	var gotRequests int

	rt := testRoundTripper(func(req *http.Request) (*http.Response, error) {
		if gotRequests == 0 {
			if hedgedhttp.IsHedgedRequest(req) {
				t.Fatal("first request is hedged")
			}
		} else {
			if !hedgedhttp.IsHedgedRequest(req) {
				t.Fatalf("%d request is not hedged", gotRequests)
			}
		}
		gotRequests++
		return nil, errors.New("just an error")
	})

	req, err := http.NewRequest(http.MethodGet, "http://no-matter-what", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	const upto = 7
	client, _, err := hedgedhttp.NewRoundTripper(10*time.Millisecond, upto, rt)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.RoundTrip(req); err == nil {
		t.Fatal(err)
	}

	if gotRequests != upto {
		t.Fatalf("want %v, got %v", upto, gotRequests)
	}
}

type testRoundTripper func(req *http.Request) (*http.Response, error)

func (t testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t(req)
}

func checkAllMetricsAreZero(tb testing.TB, metrics *hedgedhttp.Stats) {
	tb.Helper()
	expectExactMetricsAndSnapshot(tb, metrics, hedgedhttp.StatsSnapshot{})
}

func expectExactMetricsAndSnapshot(tb testing.TB, metrics *hedgedhttp.Stats, snapshot hedgedhttp.StatsSnapshot) {
	tb.Helper()
	if metrics == nil {
		tb.Fatalf("Metrics object can't be nil")
	}
	if requestedRoundTrips := metrics.RequestedRoundTrips(); requestedRoundTrips != snapshot.RequestedRoundTrips {
		tb.Fatalf("Unnexpected requestedRoundTrips: %+v; expected: %+v", requestedRoundTrips, snapshot.RequestedRoundTrips)
	}
	if actualRoundTrips := metrics.ActualRoundTrips(); actualRoundTrips != snapshot.ActualRoundTrips {
		tb.Fatalf("Unnexpected actualRoundTrips: %+v; expected: %+v", actualRoundTrips, snapshot.ActualRoundTrips)
	}
	if failedRoundTrips := metrics.FailedRoundTrips(); failedRoundTrips != snapshot.FailedRoundTrips {
		tb.Fatalf("Unnexpected failedRoundTrips: %+v; expected: %+v", failedRoundTrips, snapshot.FailedRoundTrips)
	}
	if canceledByUserRoundTrips := metrics.CanceledByUserRoundTrips(); canceledByUserRoundTrips != snapshot.CanceledByUserRoundTrips {
		tb.Fatalf("Unnexpected canceledByUserRoundTrips: %+v; expected: %+v", canceledByUserRoundTrips, snapshot.CanceledByUserRoundTrips)
	}
	if canceledSubRequests := metrics.CanceledSubRequests(); canceledSubRequests != snapshot.CanceledSubRequests {
		tb.Fatalf("Unnexpected canceledSubRequests: %+v; expected: %+v", canceledSubRequests, snapshot.CanceledSubRequests)
	}
	if currentSnapshot := metrics.Snapshot(); currentSnapshot != snapshot {
		tb.Fatalf("Unnexpected currentSnapshot: %+v; expected: %+v", currentSnapshot, snapshot)
	}
}

func testServerURL(tb testing.TB, h func(http.ResponseWriter, *http.Request)) string {
	tb.Helper()
	server := httptest.NewServer(http.HandlerFunc(h))
	tb.Cleanup(server.Close)
	return server.URL
}
