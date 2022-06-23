package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	gapi "github.com/grafana/grafana-api-golang-client"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func main() {
	// Setup args
	exemplarDashboardFile := "testing"
	baseURL := flag.String("url", "", "Base URL for grafana instance.")
	apiKey := flag.String("api-token", "", "Grafana API token.")
	flag.Parse()
	if *baseURL == "" {
		log.Fatalf("[ERROR] Failed to provide required flag 'url'")
	}
	if *apiKey == "" {
		log.Fatalf("[ERROR] Failed to provide required flag 'api-token")
	}

	// Create client
	config := gapi.Config{
		APIKey:     *apiKey,
		NumRetries: 3,
	}
	client, err := gapi.New(*baseURL, config)
	if err != nil {
		log.Fatalf("[ERROR] Failed to create grafana API client: %v", err)
	}

	// Search grafana for dashboards with exemplars enabled
	log.Println("[INFO] Searching for dashboards with exemplars")
	matchedDashboardIds := FindDashboardsWithExemplars(client)
	log.Printf("[INFO] Found %d dashboards with exemplars. Saving to file", len(matchedDashboardIds))

	// Save results to file to split out finding dashboards and changing dashboards into separate commands
	err = writeLines(matchedDashboardIds, exemplarDashboardFile)
	if err != nil {
		log.Printf("[ERROR] Failed to write file: %v", err)
	}
	log.Println("[INFO] Successfully wrote dashboard uids to file")

	// read exemplar dashboard list
	exemplarDashbordUids, err := readLines(exemplarDashboardFile)
	if err != nil {
		log.Fatalf("[ERROR] Failed to read file %s with error: %v", exemplarDashboardFile, err)
	}

	// Remove exemplars for dashboards and save
	log.Printf("[INFO] Processing %d dashbords", len(exemplarDashbordUids))
	failedTransactions, err := RemoveExemplarsFromDashboards(client, exemplarDashbordUids)
	if err != nil {
		log.Fatalf("[ERROR] Encountered unrecoverable error when running RemoveExemplarsFromDashboards operation: %v", err)
	}

	// Save failed transactions so easier to process in subsequent runs.
	if failedTransactions != nil {
		log.Printf("[INFO] Failed to remove exmplars from %d dashboards. Saving to file", len(failedTransactions))
		err = writeLines(failedTransactions, fmt.Sprintf("%s-failed-transactions", exemplarDashboardFile))
		if err != nil {
			log.Printf("[ERROR] Failed to write file %s with error: %v", fmt.Sprintf("%s-failed-transactions", exemplarDashboardFile), err)
		}
	}

	log.Printf("[INFO] Completed removing exemplars queries from dashboards. %d dashboards succesfully processed with %d failures", len(exemplarDashbordUids)-len(failedTransactions), len(failedTransactions))
}

// RemoveExemplarsFromDashboards Given a grafana API client and a list of dashboard uids representing dashboards containing exemplar queries
// this method disables the use of exemplars and updates the dashboard in grafana. If there are any dashboards
// that fail to be updated, their uids will be returned as a slice of strings. This method will continue to process all dashboards unless
// and until an unrecoverable error is encountered .
func RemoveExemplarsFromDashboards(client *gapi.Client, exemplarDashboardUids []string) ([]string, error) {
	var failedTransactions []string

	for i, dashboardUid := range exemplarDashboardUids {
		// tracking progress
		if i%5 == 0 {
			log.Printf("[INFO] Processed %d / %d dashboards", i, len(exemplarDashboardUids))
		}

		dashboard, err := client.DashboardByUID(dashboardUid)
		if err != nil {
			log.Printf("[ERROR] Failed to get dashboard from Grafana: %v", err)
			failedTransactions = append(failedTransactions, dashboardUid)
			continue
		}

		log.Printf("[INFO] Succesfully retrieved dashboard from Grafana: %v", dashboard.Meta.Slug)

		// cast dashboard model to string.
		jsonBytes, err := json.Marshal(dashboard.Model)
		if err != nil {
			log.Printf("[ERROR] Failed to get Marshal dashboard JSON: %v", err)
			failedTransactions = append(failedTransactions, dashboardUid)
			continue
		}

		// string replace exemplars: true => exemplars: false
		exemplarMatcher := regexp.MustCompile(`"exemplar":true`)
		processedModelString := exemplarMatcher.ReplaceAllString(string(jsonBytes), `"exemplar":false`)

		// UnMarshall string back to JSON.
		var processedModelJson map[string]interface{}
		err = json.Unmarshal([]byte(processedModelString), &processedModelJson)
		if err != nil {
			log.Printf("[ERROR] Failed to unmarshal processed dashboard model: %v", err)
			failedTransactions = append(failedTransactions, dashboardUid)
			continue
		}
		log.Printf("[INFO] Sucessfully disabled exemplar queries from dashboadrd: %v", dashboard.Meta.Slug)

		// Update dashboard object with new model and Save dashboard
		dashboard.Model = processedModelJson
		dashboard.Overwrite = true
		dashboardSaveResponse, err := client.NewDashboard(*dashboard)
		if err != nil {
			log.Printf("[ERROR] Failed to update processed dashboard in grafana: %v", err)
			failedTransactions = append(failedTransactions, dashboardUid)
			continue
		}

		log.Printf("[INFO] Dashboard save response from grafana: %v", dashboardSaveResponse)
	}

	return failedTransactions, nil
}

// writeLines writes the lines to the given file.
func writeLines(lines []string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

// readLines reads a whole file into memory
// and returns a slice of its lines.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// FindDashboardsWithExemplars Given a grafana api client this method queries grafana for all dashboards and returns a list
// of uids for dashboards with panels that contain exemplar queries.
func FindDashboardsWithExemplars(client *gapi.Client) []string {
	dbSearchResponses, err := client.Dashboards()
	if err != nil {
		log.Fatalf("[ERROR] Failed to get dashbosards list from Grafana: %v", err)
	}

	log.Printf("[INFO] Retrived %d dashboards", len(dbSearchResponses))
	var exemplarDashboardIds []string

	for i, dbSearchResponse := range dbSearchResponses {
		// trackking progress
		if i%5 == 0 {
			log.Printf("[INFO] Processed %d / %d dashboards", i, len(dbSearchResponses))
		}

		dashboard, err := client.DashboardByUID(dbSearchResponse.UID)
		if err != nil {
			log.Printf("[ERROR]Failed to get dashboard from Grafana: %v", err)
		}

		log.Printf("[INFO] Succesfully retrieved dashboard from Grafana: %v", dashboard.Meta.Slug)

		jsonString, err := json.Marshal(dashboard.Model)
		if err != nil {
			log.Printf("[ERROR] Failed to get Marshal dashboard JSON: %v", err)
		}

		if strings.Contains(string(jsonString), `"exemplar":true`) {
			log.Printf("[INFO] Found dashboard with exemplars: %v", dashboard.Meta.Slug)
			exemplarDashboardIds = append(exemplarDashboardIds, dashboard.Model["uid"].(string))
		}
	}

	return exemplarDashboardIds
}

// DashboardSearch Given a set of url params (as specified by the Folder dashboard search API https://grafana.com/docs/grafana/latest/developers/http_api/folder_dashboard_search/
// this method returns a list of dashboard UID's that match the params.
func DashboardSearch(client *gapi.Client, params url.Values) ([]string, error) {
	var dashboardUids []string

	log.Printf("[INFO] Searching grafana for dashboards matching params: %v", params)
	searchResponses, err := client.FolderDashboardSearch(params)
	if err != nil {
		log.Printf("[ERROR] Encountered error when making call to FolderDashboard search API: %v", err)
		return nil, err
	}

	log.Printf("[INFO] Found %d dashboard matching search query", len(searchResponses))

	for _, resp := range searchResponses {
		dashboardUids = append(dashboardUids, resp.UID)
		log.Printf("[INFO] Found dashboard matching params with uid: %s, title: %s", resp.UID, resp.Title)
	}

	return dashboardUids, nil
}
