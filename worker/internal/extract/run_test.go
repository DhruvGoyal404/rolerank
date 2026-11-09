package extract

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"testing"
	"time"
)

// A stand-in for the docker CLI: the test binary runs itself with ROLERANK_FAKE_DOCKER set.
// "create" waits FAKE_CREATE_MS, as Docker Desktop does on a busy machine; "start" reads the
// file and answers after FAKE_RUN_MS. No Docker is needed, so this runs in the normal gate.
func TestMain(m *testing.M) {
	if os.Getenv("ROLERANK_FAKE_DOCKER") == "1" {
		fakeDocker()
		return
	}
	os.Exit(m.Run())
}

func fakeDocker() {
	wait := func(env string) {
		ms, _ := strconv.Atoi(os.Getenv(env))
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	switch os.Args[1] {
	case "create":
		wait("FAKE_CREATE_MS")
		os.Stdout.WriteString("0123456789ab\n")
	case "start":
		_, _ = io.ReadAll(os.Stdin)
		wait("FAKE_RUN_MS")
		os.Stdout.WriteString(`{"kind":"pdf","pages":1,"lines":[{"n":1,"text":"ok"}],"error":""}`)
	}
	os.Exit(0) // kill and rm succeed
}

func fake(t *testing.T, createMS, runMS int) Runner {
	t.Setenv("ROLERANK_FAKE_DOCKER", "1")
	t.Setenv("FAKE_CREATE_MS", strconv.Itoa(createMS))
	t.Setenv("FAKE_RUN_MS", strconv.Itoa(runMS))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Runner{Image: "img", Docker: exe, Timeout: 1500 * time.Millisecond, StartupTimeout: 10 * time.Second}
}

var synthetic = []byte("%PDF-1.4 a synthetic resume") // the property is Docker's speed, not the file's

// Walkthrough bug (2026-11-08): Docker took longer to create the container than the extraction's
// whole limit, so a good file failed before the sandbox ran. Start-up has its own budget now.
func TestSlowContainerStartupDoesNotFailAGoodFile(t *testing.T) {
	r := fake(t, 2500, 50) // creating takes longer than the 1.5 s extraction limit
	res, err := r.Run(context.Background(), synthetic)
	if err != nil || len(res.Lines) != 1 {
		t.Fatalf("a slow start failed a good file: %v %+v", err, res)
	}
}

func TestAnExtractionPastItsLimitStillTimesOut(t *testing.T) {
	r := fake(t, 10, 4000) // the file itself runs past the 1.5 s limit
	if _, err := r.Run(context.Background(), synthetic); !errors.Is(err, ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
}

func TestADockerThatNeverStartsTheContainerSaysSo(t *testing.T) {
	r := fake(t, 5000, 10)
	r.StartupTimeout = 500 * time.Millisecond
	if _, err := r.Run(context.Background(), synthetic); !errors.Is(err, ErrStartup) {
		t.Fatalf("want ErrStartup, got %v", err)
	}
}
