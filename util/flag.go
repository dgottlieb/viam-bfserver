package util

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v61/github"
)

// Global Debug
var GDebug bool

type Arg struct {
	GithubToken  string
	JiraUsername string
	JiraToken    string

	Command     string
	IsJob       bool
	IsRun       bool
	Dedup       bool
	HandRun     bool
	FileTickets bool

	Url   string
	RunId int64
	JobId int64
}

type Set map[string]struct{}

func (mp Set) Contains(inp string) bool {
	_, exists := mp[inp]
	return exists
}

func NewSet(inps []string) Set {
	ret := make(map[string]struct{})
	for _, inp := range inps {
		ret[inp] = struct{}{}
	}

	return ret
}

func (arg *Arg) GetJiraClient() *jira.Client {
	tp := jira.BasicAuthTransport{
		Username: arg.JiraUsername,
		Password: arg.JiraToken,
	}

	jiraClient, err := jira.NewClient(tp.Client(), "https://viam.atlassian.net/")
	if err != nil {
		panic(err)
	}

	return jiraClient
}

type removeAuthOnRedirectTransport struct {
	IsRedirect bool
}

func (trans *removeAuthOnRedirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if trans.IsRedirect {
		r.Header.Del("Authorization")
		trans.IsRedirect = false
	}
	// bytes, _ := httputil.DumpRequestOut(r, true)

	resp, err := http.DefaultTransport.RoundTrip(r)
	// err is returned after dumping the response

	// respBytes, _ := httputil.DumpResponse(resp, true)
	// bytes = append(bytes, respBytes...)

	// fmt.Printf("%s\n", bytes)

	return resp, err
}

func (arg *Arg) GetGithubClient() *github.Client {
	trans := &removeAuthOnRedirectTransport{}
	httpClient := &http.Client{
		Transport: trans,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			trans.IsRedirect = true
			return nil
		},
	}

	return github.NewClient(httpClient).WithAuthToken(arg.GithubToken)
}

func ParseProgramArgs() *Arg {
	var ret Arg
	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}

	secretsFile, err := os.Open(fmt.Sprintf("%v/bfserver/secrets", configDir))
	if err != nil {
		panic(err)
	}
	defer secretsFile.Close()

	scanner := bufio.NewScanner(secretsFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			fmt.Println("Bad secrets line:", scanner.Text())
			continue
		}

		switch key {
		case "github_api_token":
			ret.GithubToken = value
		case "jira_username":
			ret.JiraUsername = value
		case "jira_api_token":
			ret.JiraToken = value
		}
	}

	commands := NewSet([]string{"analyze", "discover", "list", "test"})
	// First pass -- find the command. `os.Args` starts with the binary, e.g: `./cli`.
	for _, arg := range os.Args[1:] {
		if commands.Contains(arg) {
			ret.Command = arg
			break
		}
	}
	if ret.Command == "" {
		fmt.Println("No command", os.Args)
		os.Exit(1)
	}

	flags := map[string]*bool{
		"job":     &ret.IsJob,
		"run":     &ret.IsRun,
		"dedup":   &ret.Dedup,
		"debug":   &GDebug,
		"d":       &GDebug,
		"handRun": &ret.HandRun,
		"file":    &ret.FileTickets}
	for _, rawArg := range os.Args {
		var arg string
		switch {
		case strings.HasPrefix(rawArg, "--"):
			arg = rawArg[2:]
		case strings.HasPrefix(rawArg, "-"):
			arg = rawArg[1:]
		}

		if boolPtr, exists := flags[arg]; exists {
			*boolPtr = true
		}
	}

	lastStr := os.Args[len(os.Args)-1]
	if strings.HasPrefix(lastStr, "http") {
		ret.Url = lastStr
	}

	return &ret
}
