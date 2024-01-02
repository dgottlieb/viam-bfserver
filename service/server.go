package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/go-github/v53/github"
	"github.com/viamrobotics/bfserver/util"
)

var lastResponse *github.Response

func GithubRate() github.Rate {
	if lastResponse == nil {
		return github.Rate{}
	}
	return lastResponse.Rate
}

// Removes white-space from the end of a string
func trimRightSpace(str string) string {
	return strings.TrimRightFunc(str, unicode.IsSpace)
}

type BFServer struct {
}

func NewBFServer() *BFServer {
	return &BFServer{}
}

func FindFailingRuns(ctx context.Context, client *github.Client, startDate, endDate string) ([]*github.WorkflowRun, error) {
	service := client.Actions

	/**
	 * Workflow IDs can be retrieved via the GitHub API/CLI. E.g:
	 *   `gh workflow --repo viamrobotics/goutils list`
	 *
	 * For reference, RDK workflows have the following "Workflow IDs":
	 * - Test - 4360636
	 * - Docker - 6417489
	 * - Build AppImage - 16480684
	 * - Build and Publish Latest - 17922513
	 * - Pull Request Close - 30506204
	 * - License Finder - 31618297
	 * - License Report - 34384031
	 * - PR Test Label Manager - 38383202
	 * - Pull Request Update - 38384835
	 * - Test GCloud - 40089598
	 * - Motion Pull Request Update - 45811287
	 * - Motion Benchmark Comment on PR - 45941897
	 * - Motion Benchmarks - 46024920
	 * - Comment on PR - 46247997
	 * - NPM Publish - 48056762
	 * - .github/workflows/activate.yml - 50001502
	 * - Code Samples Pull Request Update - 50136145
	 * - Build Semi-Static Binary - 54969531
	 * - Test Binaries Cleanup - 54969532
	 * - Build and Publish RC - 56639284
	 * - Build and Publish Stable - 56642554
	 * - Bump remote-control Version - 60514228
	 */
	listOptions := github.ListWorkflowRunsOptions{
		Branch:              "main",
		ExcludePullRequests: true,
		// `Created` range query syntax:
		// https://docs.github.com/en/search-github/getting-started-with-searching-on-github/understanding-the-search-syntax#query-for-dates
		Created: fmt.Sprintf("%v..%v", startDate, endDate),
		Status:  "completed",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ret := []*github.WorkflowRun{}
	type WID struct {
		repo string
		name string
		id   int64

		onlyPush bool
	}

	// Github pagination starts at Page 1.
	for _, workflow := range []WID{
		WID{"rdk", "Build and Publish Latest", 17922513, true},
		WID{"rdk", "Docker", 6417489, false},
		WID{"rdk", "Build and Publish RC", 56639284, false},
		WID{"app", "Main Branch Update", 20902450, true},
		WID{"goutils", "Build and Test", 10703570, true},
	} {
		if util.GDebug {
			fmt.Printf("Querying: %v/%v\n", workflow.repo, workflow.name)
		}
		switch workflow.onlyPush {
		case true:
			listOptions.Event = "push"
		case false:
			listOptions.Event = ""
		}

		for page := 1; true; page++ {
			listOptions.Page = page
			workflowRuns, response, err := service.ListWorkflowRunsByID(
				ctx, "viamrobotics", workflow.repo, workflow.id, &listOptions)
			lastResponse = response
			if err != nil {
				return nil, err
			}

			for _, workflowRun := range workflowRuns.WorkflowRuns {
				if util.GDebug {
					fmt.Println("Run URL:", workflowRun.GetHTMLURL(), "Conclusion", workflowRun.GetConclusion())
				}
				if workflowRun.GetConclusion() != "failure" {
					continue
				}
				ret = append(ret, workflowRun)
			}

			if len(workflowRuns.WorkflowRuns) < listOptions.PerPage {
				break
			}
		}
	}

	return ret, nil
}

type Output struct {
	Assertions map[FQTest][]AssertionFailure
	Dataraces  map[FQTest]*DataraceFailure
	Timeouts   map[FQTest]*TimeoutFailure
	Logs       map[FQTest][]string

	PackageFailures []TestLogLine
	TestFailures    []FQTest
}

func (output *Output) IsSuccess() bool {
	return (len(output.Assertions) +
		len(output.Dataraces) +
		len(output.Timeouts) +
		len(output.PackageFailures) +
		len(output.TestFailures)) == 0
}

func (output Output) PrettyPrint(indent string) {
	for _, testFailure := range output.TestFailures {
		fmt.Println("Test Error:", testFailure)
		for _, assertion := range output.Assertions[testFailure] {
			fmt.Println(assertion.ToPrettyString(indent))
		}

		for _, logLine := range output.Logs[testFailure] {
			fmt.Println(logLine)
		}
	}

	for test, timeout := range output.Timeouts {
		fmt.Println("Timeout Error:", test)
		for _, line := range timeout.LogLines {
			fmt.Printf("%v%v", indent, line)
		}

		for _, logLine := range output.Logs[test] {
			fmt.Println(logLine)
		}
	}

	for pkg, race := range output.Dataraces {
		fmt.Println("Datarace Error:", pkg)
		for _, line := range race.LogLines {
			fmt.Printf("%v%v", indent, line)
		}

		for _, logLine := range output.Logs[pkg] {
			fmt.Println(logLine)
		}
	}

	for _, packageFailure := range output.PackageFailures {
		fmt.Println("Package Error:", packageFailure.ToPackageFailureString())
	}
}

func (output Output) ThingsThatFailed(indent string, failure Failure) {
	for test, assertions := range output.Assertions {
		for _, assertion := range assertions {
			fmt.Printf("%sFailed: %v (%v:%d)\n", indent, test, assertion.File, assertion.Line)
			fmt.Printf("%s%sCode link: %s\n",
				indent, "  ", assertion.GetAssertionCodeLink(failure.GetRepo(), failure.GitHash))
		}
	}

	for _, test := range output.Timeouts {
		fmt.Printf("%sTimeout: %v\n", indent, test)
	}

	for _, test := range output.Dataraces {
		fmt.Printf("%sDatarace: %v\n", indent, test)
	}

	fmt.Println("Debug")
	for _, test := range output.TestFailures {
		fmt.Println(test)
	}
}

func NewTestSummary() *Output {
	return &Output{
		Assertions: make(map[FQTest][]AssertionFailure),
		Dataraces:  make(map[FQTest]*DataraceFailure),
		Timeouts:   make(map[FQTest]*TimeoutFailure),
		Logs:       make(map[FQTest][]string),
	}
}

type AssertionFailure struct {
	Package  string
	File     string
	Line     int
	Expected string
	Actual   string
}

func (failure AssertionFailure) ToPrettyString(indent string) string {
	switch failure.Actual {
	case "":
		return fmt.Sprintf("%sFile:     %s/%s:%d\n%sExpected: %v\n",
			indent, failure.Package, failure.File, failure.Line,
			indent, failure.Expected)
	default:
		return fmt.Sprintf("%sFile:     %s/%s:%d\n%sExpected: %v\n%sActual:   %v",
			indent, failure.Package, failure.File, failure.Line,
			indent, failure.Expected,
			indent, strings.TrimSpace(failure.Actual))
	}
}

func (failure AssertionFailure) GetAssertionCodeLink(repo string, gitHash string) string {
	var fullName string
	switch repo {
	case "rdk":
		fullName = "go.viam.com/rdk/"
	case "goutils":
		fullName = "go.viam.com/utils/"
	case "app":
		fullName = "github.com/viamrobotics/app/"
	}
	testPkg, found := strings.CutPrefix(failure.Package, fullName)
	fmt.Printf("Package: %v Fullname: %v Test: %v Found: %v\n", failure.Package, fullName, testPkg, found)
	if !found {
		return ""
	}

	return fmt.Sprintf("https://github.com/viamrobotics/%v/blob/%s/%s/%s#L%d", repo, gitHash, testPkg, failure.File, failure.Line)
}

func (failure AssertionFailure) GetAssertionCodeLinkWithText(linkText string, runFailure Failure) string {
	return fmt.Sprintf("[%s|%s]", linkText, failure.GetAssertionCodeLink(runFailure.GetRepo(), runFailure.GitHash))
}

type DataraceFailure struct {
	Package  string
	LogLines []string
}

type TimeoutFailure struct {
	LogLines []string
}

// E.g: "    ur5e_test.go:384: Expected: nil"
// E.g: "    gpiostepper_test.go:391: Expected '0' to be between '1' and '20000' or equal to one of them (but it wasn't)!"
var expectedRe *regexp.Regexp = regexp.MustCompile(
	`^[[:space:]]*(\w+\.go):(\d+): Expected:? (.+)$`)

// E.g: "        Actual:   'timeout'"
var actualRe *regexp.Regexp = regexp.MustCompile(
	`^[[:space:]]*Actual:([^\n]+)$`)

// E.g: "panic: test timed out after 10m0s"
var startTimeoutRe *regexp.Regexp = regexp.MustCompile(
	`^panic: test timed out after (.*)$`)

// E.g: "FAIL\tgo.viam.com/rdk/services/navigation/builtin\t600.208s"
var lastTimeoutLogLineRe *regexp.Regexp = regexp.MustCompile(
	`^FAIL\t(.*)\t(.*)`)

// E.g: "Found 1 data race(s)"
var lastDataraceLogLineRe *regexp.Regexp = regexp.MustCompile(
	`Found \d+ data race\(s\)`)

func MustAtoi(digits string) int {
	ret, err := strconv.Atoi(digits)
	if err != nil {
		panic(err)
	}

	return ret
}

func parseFailures(ctx context.Context, logContents *json.Decoder) (*Output, error) {
	ret := NewTestSummary()
	allTestLogs := make(map[FQTest][]string)

	// We parse log lines one at a time, but the "expected" and "actual" values are on
	// separate log lines. Keep a buffer of any "expected" log lines missing a partner "actual".
	halfAssertionFailure := make(map[FQTest]*AssertionFailure)
	for logContents.More() {
		doc := TestLogLine{}
		err := logContents.Decode(&doc)
		if err != nil {
			return ret, err
		}
		doc.Output = trimRightSpace(doc.Output)

		if doc.Action == "fail" {
			if util.GDebug {
				fmt.Printf("Found doc.Action=`fail`.\n  Doc:%+v\n", doc)
			}
			// All failures are associated with a `Package`. Some (most) failures also are
			// associated with a `Test`. Exceptions include hangs/timeouts.
			switch doc.Test {
			case "":
				ret.PackageFailures = append(ret.PackageFailures, doc)
			default:
				// We expect test failures to be accompanied with `output` test log lines. But we
				// double-track them here as the definitive source of truth on whether a test
				// failed.
				ret.TestFailures = append(ret.TestFailures, doc.ToFQTest())
			}
			continue
		}

		if doc.Action != "output" {
			continue
		}
		allTestLogs[doc.ToFQTest()] = append(allTestLogs[doc.ToFQTest()], doc.Output)

		if matches := expectedRe.FindStringSubmatch(doc.Output); len(matches) > 0 {
			if strings.Contains(doc.Test, "TestSabertooth") {
				continue
			}
			if util.GDebug {
				fmt.Printf("Found `expected`: %v\n  Adding half-assertion for: `%v`\n",
					strings.TrimSpace(doc.Output),
					doc.ToFQTest())
				fmt.Printf("  %+v\n", doc)
			}
			if _, exists := halfAssertionFailure[doc.ToFQTest()]; exists {
				panic(fmt.Sprintf("Half assertion already existed: %v", doc.ToFQTest()))
			}

			halfAssertionFailure[doc.ToFQTest()] = &AssertionFailure{
				Package:  doc.Package,
				File:     matches[1],
				Line:     MustAtoi(matches[2]),
				Expected: matches[3],
			}
			continue
		}

		if matches := actualRe.FindStringSubmatch(doc.Output); len(matches) > 0 {
			if util.GDebug {
				fmt.Printf("Found `actual`: %v\n  Adding Assertion for: `%v`\n", doc.Output, doc.ToFQTest())
			}
			failure := halfAssertionFailure[doc.ToFQTest()]
			failure.Actual = matches[1]
			ret.Assertions[doc.ToFQTest()] = append(ret.Assertions[doc.ToFQTest()], *failure)
			ret.TestFailures = append(ret.TestFailures, doc.ToFQTest())
			delete(halfAssertionFailure, doc.ToFQTest())
			continue
		}

		// timeout stack traces can interleave with output from different tests. Keep a buffer for
		// all remaining log lines for the test.
		if startTimeoutRe.MatchString(doc.Output) {
			if util.GDebug {
				fmt.Println("Found timeout:", doc.Output)
			}
			ret.Timeouts[doc.ToFQTest()] = &TimeoutFailure{
				LogLines: []string{doc.Output},
			}
			ret.TestFailures = append(ret.TestFailures, doc.ToFQTest())
			continue
		}

		if timeoutFailure, exists := ret.Timeouts[doc.ToFQTest()]; exists {
			timeoutFailure.LogLines = append(timeoutFailure.LogLines, doc.Output)
			continue
		}

		if doc.Output == "WARNING: DATA RACE" {
			if util.GDebug {
				fmt.Println("Found data race. Package:", doc.Package, " FQTest:", doc.ToFQTest())
				fmt.Println(doc.Output)
			}
			ret.Dataraces[doc.ToFQTest()] = &DataraceFailure{
				Package:  doc.Package,
				LogLines: []string{doc.Output},
			}
			ret.TestFailures = append(ret.TestFailures, doc.ToFQTest())
			continue
		}

		if dataraceFailure, exists := ret.Dataraces[FQTest(doc.Package)]; exists {
			dataraceFailure.LogLines = append(dataraceFailure.LogLines, doc.Output)
			continue
		}
	}

	for test, expectedMsg := range halfAssertionFailure {
		if util.GDebug && !strings.Contains(string(test), "TestSabertooth") {
			fmt.Printf("Adding half assertion to full. Test: %v ExpectedMsg: %+v\n", test, *expectedMsg)
		}
		ret.Assertions[test] = append(ret.Assertions[test], *expectedMsg)
	}

	for test := range ret.Assertions {
		if util.GDebug && !strings.Contains(string(test), "TestSabertooth") {
			fmt.Println("Saving logs for assertion failure:", test)
		}
		ret.Logs[test] = allTestLogs[test]
	}
	for test := range ret.Timeouts {
		if util.GDebug {
			fmt.Println("Saving logs for timeout failure:", test)
		}
		ret.Logs[test] = allTestLogs[test]
	}
	for test := range ret.Dataraces {
		if util.GDebug {
			fmt.Println("Saving logs for datarace failure:", test)
		}
		ret.Logs[test] = allTestLogs[test]
	}

	if util.GDebug {
		fmt.Println("All failures:", ret.TestFailures)
		for _, testFailure := range ret.TestFailures {
			_, aExists := ret.Assertions[testFailure]
			_, tExists := ret.Timeouts[testFailure]
			_, dExists := ret.Dataraces[testFailure]
			if !aExists && !tExists && !dExists {
				fmt.Sprintf("Unknown test failure: %v", testFailure)
			}
		}
	}

	ret.TestFailures = sortDedupTestFailures(ret.TestFailures)

	return ret, nil
}

func sortDedupTestFailures(failures []FQTest) []FQTest {
	ret := make([]FQTest, 0)
	seen := make(map[FQTest]struct{})
	for _, failure := range failures {
		if _, exists := seen[failure]; exists {
			continue
		}

		seen[failure] = struct{}{}
		ret = append(ret, failure)
	}

	sort.Slice(ret, func(left, right int) bool {
		return ret[left] < ret[right]
	})

	return ret
}

type TestLogLine struct {
	Time string
	// One of `fail` or `output`
	Action  string
	Package string
	Output  string
	Test    string
	Elapsed float64
}

func (testLogLine TestLogLine) ToPackageFailureString() string {
	return fmt.Sprintf("%v (%vs)", testLogLine.Package, testLogLine.Elapsed)
}

type FQTest string

func (testLogLine TestLogLine) ToDataraceNamespace() FQTest {
	return FQTest(testLogLine.Package)
}

func (testLogLine TestLogLine) ToFQTest() FQTest {
	switch testLogLine.Test {
	case "":
		return FQTest(testLogLine.Package)
	default:
		return FQTest(fmt.Sprintf("%v.%v", testLogLine.Package, testLogLine.Test))
	}
}

func fetchAndParseFailures(ctx context.Context, client *github.Client, zippedLogArtifact *github.Artifact) (*Output, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", zippedLogArtifact.GetArchiveDownloadURL(), nil)
	if err != nil {
		return nil, err
	}

	response, err := client.BareDo(ctx, request)
	// Don't save `lastResponse` here. Downloading archived data does not count as an API
	// usage/return updated rate information.
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	zippedBytes := bytes.NewBuffer(make([]byte, 0, 1024))
	nCopied, err := io.Copy(zippedBytes, response.Body)
	if err != nil {
		return nil, err
	}

	if util.GDebug {
		os.WriteFile("gotest_logs.json.zip", zippedBytes.Bytes(), 0644)
	}

	archive, err := zip.NewReader(bytes.NewReader(zippedBytes.Bytes()), nCopied)
	if err != nil {
		return nil, err
	}

	testLogFile := archive.File[0]
	logContents, err := testLogFile.Open()
	if err != nil {
		return nil, err
	}
	defer logContents.Close()

	return parseFailures(ctx, json.NewDecoder(logContents))

	// ret := []TestLogLine{}
	// // Example log lines to capture:
	// //   {"Time":"2023-07-31T18:02:06.230836214Z","Action":"fail","Package":"go.viam.com/rdk/services/motion/builtin","Test":"TestMoveOnGlobe/go_around_an_obstacle","Elapsed":5.03}
	// //   {"Time":"2023-07-31T18:02:06.230876134Z","Action":"fail","Package":"go.viam.com/rdk/services/motion/builtin","Test":"TestMoveOnGlobe","Elapsed":0.01}
	// //   {"Time":"2023-07-31T18:02:09.860331074Z","Action":"fail","Package":"go.viam.com/rdk/services/motion/builtin","Elapsed":129.091}
	// doc := TestLogLine{}
	// for jsonDecoder.More() {
	//  	err = jsonDecoder.Decode(&doc)
	//  	if err != nil {
	//  		return nil, err
	//  	}
	//
	//  	if doc.Action != "fail" {
	//  		continue
	//  	}
	//
	//  	ret = append(ret, doc)
	// }
	//
	// return ret, nil
}

type Failure struct {
	Variant     string // `arm64` or `amd64`
	GithubLink  string
	GitHash     string
	Output      *Output
	WorkflowRun *github.WorkflowRun
}

// Returns the shortname. E.g: `rdk`, `goutils` or `app`.
func (failure Failure) GetRepo() string {
	return failure.WorkflowRun.GetRepository().GetName()
}

func GithubRunToFailedTests(ctx context.Context, client *github.Client, repo string, runId, jobId int64) ([]Failure, error) {
	service := client.Actions
	workflowRun, response, err := service.GetWorkflowRunByID(ctx, "viamrobotics", repo, runId)
	if err != nil {
		fmt.Println("Response:", util.ResponseBody(response.Body))
		panic(err)
		return nil, err
	}

	jobs, response, err := service.ListWorkflowJobs(ctx, "viamrobotics", repo, runId,
		// `all` and `latest`. `latest` only gives the latest re-run. Use `all` for test failures on
		// potentially prior runs. To avoid complexity of github re-runs deleting artifacts, we only
		// consider `latest` failures.
		&github.ListWorkflowJobsOptions{Filter: "latest"})
	lastResponse = response
	if util.GDebug {
		// pretty.Println("ListWorkflowRun Response:", response)
		// pretty.Println("Err:", err)
		// fmt.Println("Response:", util.ResponseBody(response.Body))
	}
	if err != nil {
		return nil, err
	}

	var jobIds struct {
		amd     int64
		arm     int64
		goutils int64
		app     int64
	}
	var errors struct {
		amd     bool
		arm     bool
		goutils bool
		app     bool
	}
	var gitHash string

	// Job names of interest:
	//   test / Build and Test (buildjet-8vcpu-ubuntu-2204, ghcr.io/viamrobotics/canon:amd64-cache, linux/amd64, ...
	//   test / Build and Test (buildjet-8vcpu-ubuntu-2204-arm, ghcr.io/viamrobotics/canon:arm64-cache, linux/arm...
	for _, job := range jobs.Jobs {
		if util.GDebug {
			fmt.Printf("Job: %v Repo: %v Conclusion: %v\n", job.GetName(), repo, job.GetConclusion())
		}

		if repo == "rdk" && !strings.Contains(job.GetName(), "Go Unit Test") && !strings.Contains(job.GetName(), "Go Coverage Test") {
			if util.GDebug {
				fmt.Println(" Skipping because rdk and not test job.")
			}
			continue
		}

		if repo == "goutils" && !strings.Contains(job.GetName(), "Build and Test") {
			if util.GDebug {
				fmt.Println(" Skipping because goutils and not test job.")
			}
			continue
		}

		if repo == "app" && !strings.Contains(job.GetName(), "test-go / Test Go") {
			if util.GDebug {
				fmt.Println(" Skipping because app and not test job.")
			}
			continue
		}

		if job.GetConclusion() != "failure" {
			if util.GDebug {
				fmt.Println(" Skipping because not failure.")
			}
			continue
		}
		if jobId != 0 && job.GetID() != jobId {
			fmt.Printf("  Skipping because not jobid. Asked: %v Received: %v\n", jobId, job.GetID())
			continue
		}

		gitHash = job.GetHeadSHA()
		switch {
		case repo == "rdk" && strings.Contains(job.GetName(), "amd64"):
			jobIds.amd = job.GetID()
			errors.amd = true
		case repo == "rdk" && strings.Contains(job.GetName(), "arm64"):
			jobIds.arm = job.GetID()
			errors.arm = true
		case repo == "goutils":
			jobIds.goutils = job.GetID()
			errors.goutils = true
		case repo == "app":
			jobIds.app = job.GetID()
			errors.app = true
		}
	}

	// Artifacts get wiped when a workflow is re-run. This is a github limitation:
	// https://github.com/actions/upload-artifact/issues/323#issuecomment-1145869465
	artifacts, response, err := service.ListWorkflowRunArtifacts(ctx, "viamrobotics", repo, runId, nil)
	lastResponse = response
	if err != nil {
		return nil, err
	}

	var logs struct {
		amd     *github.Artifact
		arm     *github.Artifact
		goutils *github.Artifact
		app     *github.Artifact
	}
	for _, artifact := range artifacts.Artifacts {
		switch {
		case repo == "rdk" && artifact.GetName() == "test-linux-amd64.json":
			logs.amd = artifact
		case repo == "rdk" && artifact.GetName() == "test-linux-arm64.json":
			logs.arm = artifact
		case repo == "goutils" && artifact.GetName() == "test.json":
			logs.goutils = artifact
		case repo == "app" && artifact.GetName() == "test.json":
			logs.app = artifact
		}
	}

	if util.GDebug {
		fmt.Printf("NumArtifacts: %v Logs: %+v\n", len(artifacts.Artifacts), logs)
		fmt.Printf("Errors: %+v\n", errors)
	}

	ret := []Failure{}
	if errors.amd == true && logs.amd != nil {
		if util.GDebug {
			fmt.Println("RDK Amd failures")
		}
		ind := NewIndenter()
		output, err := fetchAndParseFailures(ctx, client, logs.amd)
		if err != nil {
			return nil, err
		}
		if !output.IsSuccess() {
			jobLink := fmt.Sprintf("https://github.com/viamrobotics/%v/actions/runs/%v/job/%v", repo, runId, jobIds.amd)
			ret = append(ret, Failure{"amd64", jobLink, gitHash, output, workflowRun})
		}
		ind.Close()
	}

	if errors.arm == true && logs.arm != nil {
		if util.GDebug {
			fmt.Println("\nRDK Arm failures")
		}
		ind := NewIndenter()
		output, err := fetchAndParseFailures(ctx, client, logs.arm)
		if err != nil {
			return nil, err
		}
		if !output.IsSuccess() {
			// See above
			jobLink := fmt.Sprintf("https://github.com/viamrobotics/%v/actions/runs/%v/job/%v", repo, runId, jobIds.arm)
			ret = append(ret, Failure{"arm64", jobLink, gitHash, output, workflowRun})
		}
		ind.Close()
	}

	if errors.goutils == true && logs.goutils != nil {
		if util.GDebug {
			fmt.Println("\nGoutils failures")
		}
		ind := NewIndenter()
		output, err := fetchAndParseFailures(ctx, client, logs.goutils)
		if err != nil {
			return nil, err
		}
		if !output.IsSuccess() {
			// See above
			jobLink := fmt.Sprintf("https://github.com/viamrobotics/%v/actions/runs/%v/job/%v", repo, runId, jobIds.goutils)
			ret = append(ret, Failure{"goutils", jobLink, gitHash, output, workflowRun})
		}
		ind.Close()
	}

	if errors.app == true && logs.app != nil {
		if util.GDebug {
			fmt.Println("\nApp failures")
		}
		ind := NewIndenter()
		output, err := fetchAndParseFailures(ctx, client, logs.app)
		if err != nil {
			return nil, err
		}
		if !output.IsSuccess() {
			// See above
			jobLink := fmt.Sprintf("https://github.com/viamrobotics/%v/actions/runs/%v/job/%v", repo, runId, jobIds.app)
			ret = append(ret, Failure{"app", jobLink, gitHash, output, workflowRun})
		}
		ind.Close()
	}

	return ret, nil
}

func (server *BFServer) Start() {
	// ctx := context.Background()
	// client := github.NewTokenClient(ctx, githubToken)
	// failures, err := GithubRunToFailedTests(ctx, client, 5717936462)
	// fmt.Println("Rate:", lastResponse.Rate)
	// if err != nil {
	//  	panic(err)
	// }
	//
	// fmt.Println("Failures:", failures)
}
