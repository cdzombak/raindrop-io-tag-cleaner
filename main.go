package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/cdzombak/raindrop-io-api-client/pkg/raindrop"
	"golang.org/x/exp/slices"
)

const appClientId = "642d85afcadd2b30e6dff9a5"

func main() {
	ctx := context.Background()
	log.Println("-- raindrop.io tag cleaner --")

	allowlistFlag := flag.String("allowlist-file", "", "text file with allowed tags, one per line")
	dryRunFlag := flag.Bool("dry-run", false, "dry run -- print what would happen but don't delete anything")
	flag.Parse()

	clientSecret := os.Getenv("RAINDROP_CLIENT_SECRET")
	if clientSecret == "" {
		log.Fatalf("missing env variable RAINDROP_CLIENT_SECRET")
	}

	client, err := raindrop.NewClient(
		appClientId,
		clientSecret,
		"http://localhost:12705/oauth",
	)
	if err != nil {
		log.Fatalf("failed to create raindrop client: %s\n", err)
	}

	go func() {
		http.HandleFunc("/oauth", client.GetAuthorizationCodeHandler)
		if err := http.ListenAndServe(":12705", nil); err != nil {
			log.Fatalf("failed to listen for oauth callback: %s", err)
		}
	}()

	// Step 1: The authorization request
	authUrl, err := client.GetAuthorizationURL()
	if err != nil {
		log.Fatalf("failed to get raindrop auth URL: %s\n", err)
	}

	// Step 2: The redirection to your application site
	if u, err := url.QueryUnescape(authUrl.String()); err != nil {
		panic(err)
	} else {
		fmt.Printf("authorization URL: %s\n", u)
		if err := openBrowser(u); err != nil {
			log.Printf("couldn't open your browser automatically: %s\n", err)
		}
	}

	// Step 3: The token exchange
	for client.ClientCode == "" {
		log.Println("waiting for raindrop.io authorization...")
		time.Sleep(3 * time.Second)
	}

	accessTokenResp, err := client.GetAccessToken(client.ClientCode, ctx)
	if err != nil {
		log.Fatalf("failed to get access token: %s\n", err)
	}
	accessToken := accessTokenResp.AccessToken
	// end code derived from sample: https://github.com/antonnagorniy/raindrop-io-api-client

	allTags, err := client.GetTags(accessToken, ctx)
	if err != nil {
		log.Fatalf("failed to get raindrop tags: %s\n", err)
	}
	log.Printf("discovered %d Raindrop tags\n", len(allTags.Items))

	var allowlist []string
	if *allowlistFlag == "" {
		log.Println("no -allowlist-file specified.")
	} else {
		allowlistContent, err := os.ReadFile(*allowlistFlag)
		if err != nil {
			log.Fatalf("could not read allowlist file '%s': %s\n", *allowlistFlag, err)
		}
		allowlistUncleaned := strings.Split(string(allowlistContent), "\n")
		for _, v := range allowlistUncleaned {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			allowlist = append(allowlist, v)
		}
	}

	if len(allowlist) == 0 {
		fmt.Printf("no tags in allowlist; all %d tags will be deleted.\n", len(allTags.Items))
	} else {
		fmt.Printf("allowlist contains %d tags:\n\t%s\n\n", len(allowlist), strings.Join(allowlist, "\n\t"))
		toBeDeleted := len(allTags.Items) - len(allowlist)
		if toBeDeleted < 0 {
			toBeDeleted = 0
		}
		fmt.Printf("%d other tags will be deleted.\n", toBeDeleted)
	}
	fmt.Print("press 'Enter' to continue (Ctrl-C to cancel) ...")
	if _, err := bufio.NewReader(os.Stdin).ReadBytes('\n'); err != nil {
		panic(err)
	}

	successes := 0
	failures := 0

	for _, t := range allTags.Items {
		tag := t.ID
		log.Printf("processing tag '%s'...\n", tag)
		if slices.Contains(allowlist, tag) {
			log.Printf("\ttag '%s' is allowlisted; skipping it.\n", tag)
			continue
		}

		if *dryRunFlag {
			log.Printf("\tdry run: would delete tag '%s'.\n", tag)
			continue
		}

		log.Printf("\tdeleting tag '%s'...\n", tag)
		err = client.DeleteTags(accessToken, ctx, []string{tag})
		if err != nil {
			log.Printf("\tfailed: %s\n", err)
			failures++
		} else {
			log.Printf("\t✓ succeeded.\n")
			successes++
		}

		// For requests using OAuth, you can make up to 120 requests per minute per authenticated user.
		// - https://developer.raindrop.io
		time.Sleep(510 * time.Millisecond)
	}

	if *dryRunFlag {
		log.Println("dry run complete.")
	} else {
		log.Printf("complete.\nfailed to delete %d tags; deleted %d tags.\n", failures, successes)
	}
}

func openBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
