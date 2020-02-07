package blog

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gitlab.com/montebo/security"
)

var TestSite string
var DebugPrint bool
var projectId string
var MockMail *security.MockSmtpServer

const testSmtpPort = 8928
const testSmtpHostname = "127.0.0.1"

func TestMain(m *testing.M) {
	var err error

	MockMail, err = security.NewMockSmtpServer(testSmtpHostname, testSmtpPort)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Start datastore emulator
	cmd := exec.Command("gcloud", "beta", "emulators", "datastore", "start", "--no-store-on-disk")
	stdout, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Failed creating pipe to datastore emulator.", err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("Failed starting datastore emulator.", err)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
	host := ""
	for scanner.Scan() {
		m := scanner.Text()
		if m == "[datastore] Dev App Server is now running." {
			fmt.Println(m)
			break
		}
		i := strings.Index(m, "DATASTORE_EMULATOR_HOST=")
		if i > 0 {
			host = m[i+24:]
			fmt.Println(m)
			fmt.Println(host + ".")
			fmt.Println()
		}
	}
	projectId = host

	// setup
	TestSite = security.RandomString(10) + ".com"
	DebugPrint = os.Getenv("DEBUG") != ""

	defer func() {
		MockMail.Stop()

		// cleanup
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
	}()

	code := m.Run()

	fmt.Println("code:", code, "pid:", cmd.Process.Pid)

	return
}

func requireEnv(name string, t *testing.T) string {
	if name == "GOOGLE_CLOUD_PROJECT" {
		return projectId
	}
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf(fmt.Sprintf("Environment variable required: %s", name))
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

func SetupBasicConnectorData(t *testing.T) {
	am, err, _, _ := security.NewGaeAccessManager(projectId, inferLocation(t), time.Now().Location())
	if err != nil {
		t.Fatalf("NewGaeAccessManager() failed: %v", err)
	}

	am.Setting().Put(TestSite, "self.signup", "no")
	am.Setting().Put(TestSite, "smtp.hostname", testSmtpHostname)
	am.Setting().Put(TestSite, "smtp.port", fmt.Sprintf("%d", testSmtpPort))
	am.Setting().Put(TestSite, "support_team.name", "SUPPORT_TEAM_NAME")
	am.Setting().Put(TestSite, "support_team.email", "SUPPORT_TEAM_EMAIL@example.com")
	//am.Setting().Put(TestSite, "smtp.user", requireEnv("SMTP_USER", t))
	//am.Setting().Put(TestSite, "smtp.password", requireEnv("SMTP_PASSWORD", t))

	_, err = am.AddPerson(TestSite, "connector_setup", "tmp", "connector_setup@example.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("tmp1!aAfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}
	_, err = am.AddPerson(TestSite, "James", "Smith", "james@montebo.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("3axf.qQfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}
	time.Sleep(time.Millisecond * 10)
	_, _, err = am.Authenticate(TestSite, "connector_setup@example.com", "tmp1!aAfo", "127.0.0.1", "", "en-AU")
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}

	fmt.Println("Test data loaded")
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
