// Command suggest-version is the entry point of the ArtefactFlow suggest-version GitHub Action. It reads the action inputs from the environment, asks the ArtefactFlow API for the suggested next versions of one artefact, and writes them to $GITHUB_OUTPUT for later steps to consume.
//
// It computes nothing itself: the API owns the SemVer logic.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/artefactflow/suggest-version-action/internal/action"
	"github.com/artefactflow/suggest-version-action/internal/afclient"
)

// version is stamped at build time with -ldflags. It is only used for the summary line in the step log.
var version = "dev"

func main() {
	// Every failure surfaces as an ::error:: annotation so it lands on the workflow run's summary page, not only in the collapsed step log.
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "::error::%s\n", sanitiseAnnotation(err.Error()))
		os.Exit(1)
	}
}

func run() error {
	// The runner cancels a job by signalling the container; honouring that stops an in-flight retry loop promptly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cfg, err := action.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}

	// The service account presents its client credentials and gets a short-lived token back. It is used for the calls below and never stored, logged, or emitted as an output.
	token, err := afclient.Authenticate(ctx, cfg.APIURL, cfg.ClientID, cfg.ClientSecret)
	if err != nil {
		return err
	}
	// Mask it before it is used anywhere: the runner only masks values it was given as secrets, and a minted token is not one of them. Without this, a later step running with debug logging could print it in the clear.
	//
	// Only under a runner, though. Off a runner nothing interprets the workflow command, so it would print the token verbatim to a terminal or a captured log — the opposite of the intent.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Printf("::add-mask::%s\n", token)
	}
	fmt.Printf("Authenticated service account %s\n", cfg.ClientID)

	client, err := afclient.New(cfg.APIURL, token)
	if err != nil {
		return err
	}

	// The versions endpoint addresses an artefact by id, so the name is resolved first. The id is never asked for or emitted — a workflow only ever deals in names.
	artefactID, err := client.ResolveArtefactID(ctx, cfg.ArtefactName)
	if err != nil {
		return err
	}
	fmt.Printf("Resolved artefact %s to %s\n", cfg.ArtefactName, artefactID)

	suggestions, err := client.SuggestedVersions(ctx, artefactID)
	if err != nil {
		return err
	}

	outputs, err := action.Outputs(suggestions, cfg.Bump)
	if err != nil {
		return err
	}

	if err := writeOutputs(outputs); err != nil {
		return err
	}

	// A one-line summary makes the common "which version did it pick?" question answerable without expanding anything.
	fmt.Printf("suggest-version %s: latest=%s, selected %s version %s\n", version, orNone(suggestions.Latest), cfg.Bump, outputs[0].Value)
	return nil
}

// writeOutputs appends the outputs to the file the runner points $GITHUB_OUTPUT at. Running outside a runner (a local smoke test) is not an error — the outputs go to stdout instead.
func writeOutputs(outputs []action.Output) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		fmt.Fprintln(os.Stdout, "GITHUB_OUTPUT is not set; printing outputs instead:")
		return action.WriteOutputs(os.Stdout, outputs)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("open GITHUB_OUTPUT: %w", err)
	}
	defer f.Close()
	if err := action.WriteOutputs(f, outputs); err != nil {
		return err
	}
	return f.Close()
}

// sanitiseAnnotation flattens an error onto one line. A workflow command is line-oriented, so an embedded newline would truncate the annotation and leak the remainder as raw log text.
func sanitiseAnnotation(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
