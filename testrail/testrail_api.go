package testrail

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const (
	TestRailProjectID = 34 // project id from Test Rail
	TestRailSuiteID   = 5279 //  suiteID from Test Rail
)

var (
	testCaseMap      map[string]int
	testRailPassword string = os.Getenv("TESTRAIL_PASSWORD")
	testRailBaseUrl  string = os.Getenv("ORBITAL_TEST_RAIL")
	testRailUser     string = os.Getenv("TESTRAIL_USER")
	InfoLogger       zerolog.Logger
	//testSuiteMap     = make(map[string]int)
)

type Run struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Case struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type TestsResponse struct {
	Cases []Case `json:"cases"`
}

type Result struct {
	CaseID       int    `json:"case_id"`
	StatusID     int    `json:"status_id"`
	Comment      string `json:"comment,omitempty"`
	Version      string `json:"version,omitempty"`
	Elapsed      string `json:"elapsed,omitempty"`
	Defects      string `json:"defects,omitempty"`
	AssignedToID int    `json:"assignedto_id,omitempty"`
}

type Results struct {
	Results []Result `json:"results"`
}

// Struct for a Project
type Project struct {
	Announcement     string `json:"announcement"`
	CompletedOn      int    `json:"completed_on"`
	ID               int    `json:"id"`
	IsCompleted      bool   `json:"is_completed"`
	Name             string `json:"name"`
	ShowAnnouncement bool   `json:"show_announcement"`
	SuiteMode        int    `json:"suite_mode"`
	URL              string `json:"url"`
}

type Sections struct {
	Sections []Section `json:"sections"`
}

// Struct for a get Section response
type Section struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Suites struct {
	Suites []Suite `json:"suites"`
}

// Struct for a get Suite response
type Suite struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Struct to match the CSV structure
type TestResult struct {
	Title       string `json:"title"`
	Status      string `json:"type_id"`
	PriorityID  string `json:"priority_id"`
	Estimate    string `json:"estimate"`
	CustomOS    string `json:"custom_operating_system"`
	Description string `json:"custom_test_case_description"`
}

func CreateTestCaseIdMap(tests []Case) {
	testCaseMap = make(map[string]int)
	for _, t := range tests {
		testCaseMap[t.Title] = t.ID
	}
}

func GetTestCaseIdByName(testCaseName string) (int, bool) {
	caseId, validKey := testCaseMap[testCaseName]
	return caseId, validKey
}

// Convert Testcases in CSV to JSON
func CsvtoJson(filePath string) ([]string, error) {
	var jsonRows []string
	// Open the CSV file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening CSV file:", err)
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))

	for {
		line, error := reader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			fmt.Println("Error reading CSV line:", error)
			return nil, error
		}

		// Convert CSV row to TestResult struct
		testResult := TestResult{
			Title:       line[0],
			Status:      line[1],
			PriorityID:  line[2],
			Estimate:    line[3],
			CustomOS:    line[4],
			Description: line[5],
		}

		jsonData, err := json.Marshal(testResult)
		if err != nil {
			fmt.Println("Error converting to JSON:", err)
			continue
		}
		jsonRows = append(jsonRows, string(jsonData))
	}
	return jsonRows, nil
}

// Get Section ID when section name , project id and suite id is passed.
func GetSectionIDByName(sectionName string, ProjectID int, SuiteID int) (int, bool) {
	url := fmt.Sprintf("%s/index.php?/api/v2/get_sections/%d&suite_id=%d", testRailBaseUrl, ProjectID, SuiteID)
	fmt.Println(`url:`, url)

	// Send POST to TestRail URL to Get section id for provided section name
	statusCode, resp := executeTestRailRequest("GET", url, bytes.NewReader([]byte(``)))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on posting test cases: %d\n", statusCode)
	}
	// unmarshal response into sections struct
	var sections Sections
	json.Unmarshal(resp, &sections)
	if len(sections.Sections) == 0 {
		//section is empty
		return -1, false
	}
	// find section id for provided section name
	var sectionId int
	for _, section := range sections.Sections {
		fmt.Println(`section:`, section)
		if section.Name == sectionName {
			sectionId = section.ID
		}
	}
	validKey := sectionId != 0
	fmt.Println("sectionId ", sectionId)
	fmt.Println("validkey ", validKey)
	return sectionId, validKey
}

// Get suiteID for the SuiteName in a given project
func GetSuiteIDByName(SuiteName string, TestRailProjectID int) (int, error) {
	//GET index.php?/api/v2/get_suites/{project_id}
	var SuiteID int
	var Suites []Suite
	url := fmt.Sprintf("%s/index.php?/api/v2/get_suites/%d", testRailBaseUrl, TestRailProjectID)
	fmt.Println(url)
	// Send POST to TestRail URL to Get suite id for provided project name
	statusCode, resp := executeTestRailRequest("GET", url, bytes.NewReader([]byte(``)))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on posting test cases: %d\n", statusCode)
	}
	fmt.Println(statusCode, string(resp))
	// unmarshal response into suites struct
	json.Unmarshal(resp, &Suites)
	fmt.Println(len(Suites))
	if len(Suites) == 0 {
		//suites list is empty
		return -1, errors.New("Suites list is empty")
	}
	for _, suite := range Suites {
		fmt.Println(suite.Name)
		if suite.Name == SuiteName {
			SuiteID = suite.ID
		}
	}
	if SuiteID == 0 {
		return -1, errors.New("Suite ID not found, for suite name " + SuiteName)
	}
	return SuiteID, nil
}

// upload tests in csv in a given path, to the section name in the request.
func UploadTestCasesInCSV(csvFilePath string, sectionName string) (statusCode int, resp []byte, err error) {
	TestJson, error := CsvtoJson(csvFilePath)
	if error != nil {
		fmt.Println("Cannot read the csv file, ensure there exists a csv file", error)
		return -1, nil, error
	}
	sectionID, _ := GetSectionIDByName(sectionName, TestRailProjectID, TestRailSuiteID)
	if sectionID == -1 {
		return -1, nil, errors.New("Section ID not found")
	}
	if len(TestJson) > 0 {
		for i, row := range TestJson {
			fmt.Println(TestJson[i])
			url := fmt.Sprintf("%s/index.php?/api/v2/add_case/%d", testRailBaseUrl, 266771)
			fmt.Printf("url to create testcase: %s\n", url)
			statusCode, resp := executeTestRailRequest("POST", url, bytes.NewReader([]byte(row)))
			if statusCode != http.StatusOK {
				fmt.Printf("Received non-200 Status Code on posting test cases: %d\n", statusCode)
			}
			fmt.Printf("Response: %s", string(resp))
		}
	} else {
		fmt.Println("No test cases to upload")
		return -1, nil, errors.New("no test cases to upload")
	}

	fmt.Println("Test cases uploaded successfully")
	return statusCode, resp, nil
}

// Get TestRail Project
func GetProject(projectId int) Project {
	url := fmt.Sprintf("%s/index.php?/api/v2/get_project/%v", testRailBaseUrl, projectId)

	statusCode, resp := executeTestRailRequest("GET", url, bytes.NewReader([]byte(``)))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on fetching TestRail project: %d\n", statusCode)
	}

	var project Project
	err := json.Unmarshal(resp, &project)
	if err != nil {
		fmt.Printf("TestRail project failed to unmarshal: %v\n", err)
	}

	return project
}

func executeTestRailRequest(method string, url string, payload *bytes.Reader) (statusCode int, resp []byte) {
	req, _ := http.NewRequest(method, url, payload)
	req.SetBasicAuth(testRailUser, testRailPassword)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	tlsConfig := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	httpClient := &http.Client{Transport: tlsConfig}

	response, err := httpClient.Do(req)
	if err != nil {
		InfoLogger.Print("Error Posting the Request", err)
	}

	respBytes, readerr := io.ReadAll(response.Body)
	if readerr != nil {
		fmt.Printf("Error reading the response body %v", readerr)
	}

	return response.StatusCode, respBytes
}

func CreateTestRunWithCaseIds(envName string, projectId int, suiteID int, caseIds *[]int, description string) (runId int) {
	url := fmt.Sprintf("%s/index.php?/api/v2/add_run/%d", testRailBaseUrl, projectId)

	var runName string
	if envName != "" {
		runName = fmt.Sprintf("orbitalqa-run-%s-%v", envName, time.Now().Unix())
	} else {
		runName = fmt.Sprintf("orbitalqa-run-%v", time.Now().Unix())
	}

	body := fmt.Sprintf(`{
		"suite_id":%d,
		"name": "%s",
		"include_all": false,
		"case_ids": %v,
		"description": "%v"
	}`, suiteID, runName, buildCaseIdString(*caseIds), sanitizeString(description))

	statusCode, resp := executeTestRailRequest("POST", url, bytes.NewReader([]byte(body)))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on creating test run: %v with a resp: %v\n", statusCode, string(resp))
	}

	var run Run
	err := json.Unmarshal(resp, &run)
	if err != nil {
		fmt.Printf("Test Run failed to unmarshal: %v\n", err)
	}

	return run.ID
}

func buildCaseIdString(caseIds []int) string {
	return strings.Replace(fmt.Sprint(caseIds), " ", ", ", -1)
}

func sanitizeString(s string) string {
	// Protect tabs and new lines in the description string
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func GetTestsByProjectId(projectId int, suiteId int) []Case {
	url := fmt.Sprintf("%s/index.php?/api/v2/get_cases/%d&suite_id=%d", testRailBaseUrl, projectId, suiteId)
	fmt.Println("Inside gettestsby project : " + url)

	statusCode, resp := executeTestRailRequest("GET", url, bytes.NewReader([]byte(``)))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on fetching cases from TestRail project: %d\n", statusCode)
		fmt.Println(string(resp))
	}

	var testsResponse TestsResponse
	err := json.Unmarshal(resp, &testsResponse)
	if err != nil {
		fmt.Printf("Tests failed to unmarshal: %v\n", err)
	}
	return testsResponse.Cases
}

func AddResultsForTestCases(runId int, results Results) {
	url := fmt.Sprintf("%s/index.php?/api/v2/add_results_for_cases/%d", testRailBaseUrl, runId)

	resultsJson, err := json.Marshal(results)
	if err != nil {
		fmt.Printf("Test results failed to marshal: %v\n", err)
		return
	}

	statusCode, _ := executeTestRailRequest("POST", url, bytes.NewReader(resultsJson))
	if statusCode != http.StatusOK {
		fmt.Printf("Received non-200 Status Code on posting test results: %d\n", statusCode)
		return
	}
}

func PatternExists(pattern string, patterns []string) bool {
	for _, p := range patterns {
		if p == pattern {
			return true
		}
	}
	return false
}

// Scan a File that have the scripted tests, extract the tests and description , create a testcase csv that can be uploaded to testrail
func GenerateTestcaseCSV(dir string, outputFile string) error {
	const (
		status     int    = 1
		priorityid int    = 3
		estimate   string = "3m"
		customos   int    = 1 //default to windows
	)
	// Open the CSV file for writing
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Regex to match test function names and comments
	funcRegex := regexp.MustCompile(`func (Test\w+)\(`)
	commentRegex := regexp.MustCompile(`(?s)(/\*.*?\*/)\s*func\s+\w+\(`)
	isMac := regexp.MustCompile(`(?i)(?:osx|darwin|macos|macosx)`)
	isLinux := regexp.MustCompile(`(?i)(?:linux|unix|amazon|centos|ubuntu|debian|fedora|opensuse|rhel)`)

	// Walk through the directory
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "_test.go") {
			// Read the file
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lines := strings.Split(string(content), "\n")
			var description string
			var customos int = 1
			for _, line := range lines {
				if commentMatch := commentRegex.FindStringSubmatch(line); commentMatch != nil {
					description += commentMatch[1]
					fmt.Println(description)
				}
				if commentMatch := isMac.FindStringSubmatch(line); commentMatch != nil {
					customos = 2
				} else if commentMatch := isLinux.FindStringSubmatch(line); commentMatch != nil {
					customos = 3
				}
				if funcMatch := funcRegex.FindStringSubmatch(line); funcMatch != nil {
					funcName := funcMatch[1]
					// Write the function name and last found comment to the CSV
					escapedDescription := strings.ReplaceAll(description, `"`, `""`)
					csvLine := fmt.Sprintf(`"%s",%d,%d,"%s",%d,"%s":"%s"\n`, funcName, status, priorityid, estimate, customos, info.Name(), escapedDescription)
					if _, err := writer.WriteString(csvLine); err != nil {
						return err
					}
					//if _, err := writer.WriteString(fmt.Sprintf("%s,%d,%d,%s,%d,%s\n", funcName, status, priorityid, estimate, customos, info.Name()+":"+description)); err != nil {
					//	return err
					//}
					description = "" // Reset last comment
					customos = 1     //Reset customos
				}
			}
		}
		return nil
	})
	return err
}

// Reads golang source code in a given directory, parses it and creates a csv file with tests and its descriptions, that
// can be directly uploaded to testrail via api.
func ExtractTestsAndCommentsToCSV(dir string, outputFile string) error {
	var (
		status     = 1
		priorityid = 3
		estimate   = "3m"
		defaultOS  = 1 // default to windows
	)
	// Open the CSV file for writing
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Regex to match test function names and comments
	isMac := regexp.MustCompile(`(?i)(?:osx|darwin|macos|macosx)`)
	isLinux := regexp.MustCompile(`(?i)(?:linux|unix|amazon|centos|ubuntu|debian|fedora|opensuse|rhel)`)

	// Walk through the directory using os.WalkDir
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Error walking directory: ", err)
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), "_test.go") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			var customos = defaultOS
			fmt.Printf("Extracting function names from file: %s , %s", d.Name(), path)
			fns, err := extractFunc(path)
			if err != nil {
				fmt.Printf("Error extracting function names from file: %s , error: %s\n", path, err)
				return err
			}
			// filter out the tests that begin with 'Test'
			re := regexp.MustCompile(`^(Test)`)
			for _, fn := range fns {
				if !re.MatchString(fn) {
					continue
				}
				fmt.Printf("Extracted function name: %s\n", fn)
				// here get the map and separate call is not needed
				comment, err := extractCommentsAboveFunc(string(content), fn)
				fmt.Printf("Extracted comment for function: %s\n", comment)
				if err != nil {
					fmt.Println("Error extracting comments:", err)
					return err
				}
				if commentMatch := isMac.FindStringSubmatch(comment); commentMatch != nil {
					customos = 2
				} else if commentMatch := isLinux.FindStringSubmatch(comment); commentMatch != nil {
					customos = 3
				}
				escapedComment := strings.ReplaceAll(comment, `"`, `""`)
				//csvLine := fmt.Sprintf("%s,%d,%d,%s,%d,%s\n", fn, status, priorityid, estimate, customos, d.Name())
				csvLine := fmt.Sprintf("%s,%d,%d,%s,%d,%s\n", fn, status, priorityid, estimate, customos, sanitizeString(escapedComment))
				if _, err := writer.WriteString(csvLine); err != nil {
					return err
				}
				//reset the os to windows
				customos = 1
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return writer.Flush()
}

// reurns a list of function names in the .go source file.
func extractFunc(filePath string) ([]string, error) {
	fset := token.NewFileSet()
	//Parse the source file
	f, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil, err
	}
	var functionNames []string
	ast.Inspect(f, func(n ast.Node) bool {
		// Check if the node is a function declaration.
		if fn, ok := n.(*ast.FuncDecl); ok {
			// Append the function name to the list.
			functionNames = append(functionNames, fn.Name.Name)
		}
		return true // continue traversing the AST
	})

	return functionNames, nil
}

// searches for comments directly above the function named funcName.returns the comments as string.
func extractCommentsAboveFunc(src, funcName string) (string, error) {
	fset := token.NewFileSet() // positions are relative to fset

	// Parse the source file.
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return "", err
	}

	var commentsString string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if fn.Name.Name == funcName {
			if fn.Doc != nil {
				for _, comment := range fn.Doc.List {
					commentsString += strings.TrimSpace(comment.Text) + "\n"
				}
			}
		}
		return true
	})

	return commentsString, nil
}

// parseSource parses the given Go source code and extracts test names,
// including names of nested tests.
func parseSource(src string) []string {
	// Parse the source code into an AST.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		panic(err)
	}

	// Prepare a slice to hold the names of the tests.
	var tests []string

	// Traverse the AST.
	ast.Inspect(file, func(n ast.Node) bool {
		// Look for function declarations.
		if fn, ok := n.(*ast.FuncDecl); ok {
			// Check if the function is a test function.
			if fn.Name.Name[:4] == "Test" {
				testName := fn.Name.Name
				tests = append(tests, testName) // Add the test function name.

				// Look for nested t.Run calls.
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if callExpr, ok := n.(*ast.CallExpr); ok {
						if selectorExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
							if selectorExpr.Sel.Name == "Run" {
								if lit, ok := callExpr.Args[0].(*ast.BasicLit); ok {
									subTestName := lit.Value
									subTestName = subTestName[1 : len(subTestName)-1]
									fullName := fmt.Sprintf("%s/%s", testName, subTestName)
									tests = append(tests, fullName) // Add the nested test name.

									// TODO: Handle deeper nesting if necessary.
									// TODO: fetch the Doc for each test in tests, and map it to the test
								}
							}
						}
					}
					return true
				})
			}
		}
		return true
	})
	// tests needs to be a map key=test value=comment extracted
	return tests
}

/*
func main() {
	fmt.Println("TestRail Upload")

	SuiteID, err := GetSuiteIDByName("SMOKETEST_PROBEAPI", 34)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("suiteID ", SuiteID)


	// Create a csv for a given folder
	error := ExtractTestsAndCommentsToCSV("/Users/soupanda/orbital/crater/api", "testCasesCrater.csv")
	if error != nil {
		fmt.Println(error)
	} else {
		fmt.Println("testCases.csv created")
	}

	// Upload cases to TestRail
	//status, resp, err := UploadTestCasesInCSV("/Users/soupanda/orbital/qa/test_rail/testCasesCrater.csv", "SMOKETEST_PROBEAPI")
	//if err != nil {
	//	fmt.Println(err)
	//}
	//fmt.Println(status, string(resp))
	//fmt.Println("Test cases uploaded successfully")
}
*/
