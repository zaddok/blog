package blog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/datastore"
	"gitlab.com/montebo/security"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func dumpSystemLog(am security.AccessManager, site string) {
	session, _ := am.GetSystemSession(site, "Test", "Test")
	items, _ := am.GetRecentSystemLog(session)
	for _, i := range items {
		fmt.Println(i)
	}
}

var TestSite string
var TestCqlKeyspace string
var DebugPrint bool
var projectId string
var mockSmtpServer *security.MockSmtpServer
var testCassandraHostname string = "127.0.0.1"

const testSmtpPort = 8928
const testSmtpHostname = "127.0.0.1"

func TestMain(m *testing.M) {
	if os.Getenv("CASSANDRA_HOSTNAME") != "" {
		testCassandraHostname = os.Getenv("CASSANDRA_HOSTNAME")
		fmt.Println("Cassandra hostname: ", testCassandraHostname)
	}

	//fmt.Println("starting mock smtp server")
	srv, err := security.NewMockSmtpServer(testSmtpHostname, testSmtpPort)
	if err != nil {
		fmt.Println(err)
		return
	}
	mockSmtpServer = srv

	// Start datastore emulator
	//fmt.Println("starting datastore emulator")
	cmd := exec.Command("gcloud", "beta", "emulators", "datastore", "start", "--no-store-on-disk", "--consistency=1.0", "--quiet")
	if err != nil {
		fmt.Println("Failed creating pipe to datastore emulator.", err)
		os.Exit(1)
	}

	stdout, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Failed creating pipe to datastore emulator.", err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("Failed starting datastore emulator.", err)
		os.Exit(1)
	}
	//fmt.Println("watching datastore emulator output")
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
	host := ""
	output := ""
	for scanner.Scan() {
		m := scanner.Text()
		output = output + m + "\n"
		if m == "[datastore] Dev App Server is now running." {
			//fmt.Println(m)
			break
		}
		i := strings.Index(m, "DATASTORE_EMULATOR_HOST=")
		if i > 0 {
			host = m[i+24:]
			//fmt.Println(m)
			//fmt.Println(host + ".")
			//fmt.Println()
		}
	}
	if host == "" {
		fmt.Println("Failed reading host from gcloud datastore emulator")
		fmt.Println(output)
		return
	}
	projectId = host

	// setup
	TestSite = security.RandomString(10) + ".com"
	TestCqlKeyspace = "test_" + security.RandomString(10)
	DebugPrint = os.Getenv("DEBUG") != ""

	code := m.Run()

	// cleanup

	if DebugPrint {
		fmt.Println("Dumping records")
		var client *datastore.Client
		ctx := context.Background()
		if strings.LastIndex(projectId, ":") > 0 {
			client, err = datastore.NewClient(ctx, "test", option.WithEndpoint(projectId),
				option.WithoutAuthentication(),
				option.WithGRPCDialOption(grpc.WithInsecure()))
		} else {
			client, err = datastore.NewClient(ctx, projectId)
		}
		if err != nil {
			fmt.Printf("Failed to create client: %v", err)
		}

		q := datastore.NewQuery("__namespace__").KeysOnly()
		namespaces, err := client.GetAll(ctx, q, nil)
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		for _, ns := range namespaces {
			q := datastore.NewQuery("").Namespace(ns.Name).KeysOnly().Limit(500)
			keys, err := client.GetAll(ctx, q, nil)
			if err != nil {
				fmt.Printf("%v\n", err)
				return
			}
			for _, k := range keys {
				fmt.Printf("  %s key: %v\n", ns.Name, k)
			}
		}
	}

	url := "http://" + host + "/shutdown"
	req, err := http.NewRequest("POST", url, nil)
	b := &http.Client{}
	resp, err := b.Do(req)
	if err != nil {
		fmt.Println("Failed POST'ing shutdown request to datastore emulator", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Println(string(body))

	srv.Stop()

	os.Exit(code)
}

func requireEnv(name string, t *testing.T) string {
	if name == "GOOGLE_CLOUD_PROJECT" {
		return projectId
	}

	value := os.Getenv(name)
	if value == "" {
		fmt.Println("Test environment not configured properly. " + name + " not set.")
	}
	return value
}

func inferLocation(t *testing.T) string {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	location := "australia-southeast1"
	if project == "sis-us" {
		location = "us-west2"
	}
	return location
}

func StringToDatePointer(text string) *time.Time {
	t, _ := AttemptDateParse(text, 0)
	return t
}

func AttemptDateParse(text string, tabYear int) (*time.Time, error) {
	text = strings.TrimSpace(text)

	if strings.HasSuffix(text, " 00:00:00") {
		text = text[0 : len(text)-9]
	}

	if text == "" || text == "-" || text == "n/a" || text == "N/A" {
		return nil, nil
	}

	if len(text) > 17 && ((text[0] == '2' && (text[1] == '0' || text[1] == '1')) || (text[0] == '1' && text[1] == '9')) {
		v, err := time.Parse("2006-1-2 15:04:05", text)
		if err == nil {
			return &v, nil
		}
	}

	if len(text) > 7 && ((text[0] == '2' && (text[1] == '0' || text[1] == '1')) || (text[0] == '1' && text[1] == '9')) {
		v, err := time.Parse("2006-1-2", text)
		if err == nil {
			return &v, nil
		}
		v, err = time.Parse("2006/1/2", text)
		if err == nil {
			return &v, nil
		}
	}

	if strings.Index(text, "-") > 0 {
		v, err := time.Parse("2-1-2006", text)
		if err == nil {
			return &v, nil
		}
		v, err = time.Parse("2-1-6", text)
		if err == nil {
			return &v, nil
		}
		v, err = time.Parse("2-Jan-2006", text)
		if err == nil {
			return &v, nil
		}
	}

	if strings.Index(text, "/") > 0 {
		v, err := time.Parse("2/1/2006", text)
		if err == nil {
			return &v, nil
		}
		v, err = time.Parse("2/1/06", text)
		if err == nil {
			return &v, nil
		}
	}

	if tabYear > 0 && len(strings.FieldsFunc(text, func(c rune) bool { return c == '/' || c == ' ' })) == 2 {
		v, err := time.Parse("2/1/2006", fmt.Sprintf("%s/%d", text, tabYear))
		if err == nil {
			return &v, nil
		}
	}

	return nil, errors.New("Unknown date format")
}
