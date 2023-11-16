package _select

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pgollangi/fireql/pkg/util"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"cloud.google.com/go/firestore"
)

type TestExpect struct {
	query   string
	columns []string
	length  string
	records [][]interface{}
}

const FirestoreEmulatorHost = "FIRESTORE_EMULATOR_HOST"

var selectTests = []TestExpect{
	//TestExpect{
	//	query:   "select * from users",
	//	columns: []string{"id", "email", "username", "address", "name"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select * from `users`",
	//	columns: []string{"id", "email", "username", "address", "name"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select id as uid, * from users",
	//	columns: []string{"uid", "id", "email", "username", "address", "name"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select *, username as uname from users",
	//	columns: []string{"id", "email", "username", "address", "name", "uname"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select  id as uid, *, username as uname from users",
	//	columns: []string{"uid", "id", "email", "username", "address", "name", "uname"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select id, email, address from users",
	//	columns: []string{"id", "email", "address"},
	//	length:  "21",
	//},
	//TestExpect{
	//	query:   "select id, email, address from users limit 5",
	//	columns: []string{"id", "email", "address"},
	//	length:  "5",
	//},
	//TestExpect{
	//	query:   "select id from users where email='aeatockj@psu.edu'",
	//	columns: []string{"id"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{float64(20)}},
	//},
	//TestExpect{
	//	query:   "select id from users order by id desc limit 1",
	//	columns: []string{"id"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{float64(21)}},
	//},
	//TestExpect{
	//	query:   "select LENGTH(username) as uLen from users where id = 8",
	//	columns: []string{"uLen"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{float64(6)}},
	//},
	//TestExpect{
	//	query:   "select id from users where `address.city` = 'Glendale' and name = 'Eleanora'",
	//	columns: []string{"id"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{float64(10)}},
	//},
	//TestExpect{
	//	query:   "select id > 0 as has_id from users where `address.city` = 'Glendale' and name = 'Eleanora'",
	//	columns: []string{"has_id"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{true}},
	//},
	//TestExpect{
	//	query:   "select __name__ from users where id = 1",
	//	columns: []string{"__name__"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{"1"}},
	//},
	//TestExpect{
	//	query:   "select id, email, username from users where id = 21",
	//	columns: []string{"id", "email", "username"},
	//	length:  "1",
	//	records: [][]interface{}{[]interface{}{float64(21), nil, "ckensleyk"}},
	//},
	TestExpect{
		query:   "select id from users where email != null",
		columns: []string{"id", "email", "username"},
		length:  "1",
		records: [][]interface{}{[]interface{}{float64(21)}},
	},
}

func newFirestoreTestClient(ctx context.Context) *firestore.Client {
	client, err := firestore.NewClient(ctx, "test")
	if err != nil {
		log.Fatalf("firebase.NewClient err: %v", err)
	}

	return client
}

func TestMain(m *testing.M) {
	// command to start firestore emulator
	cmd := exec.Command("gcloud", "beta", "emulators", "firestore", "start", "--host-port=localhost:8765")

	// this makes it killable
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// we need to capture it's output to know when it's started
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	defer stderr.Close()

	// start her up!
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// ensure the process is killed when we're finished, even if an error occurs
	// (thanks to Brian Moran for suggestion)
	var result int
	defer func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		os.Exit(result)
	}()

	// we're going to wait until it's running to start
	var wg sync.WaitGroup
	wg.Add(1)

	// by starting a separate go routine
	go func() {
		// reading it's output
		buf := make([]byte, 256, 256)
		for {
			n, err := stderr.Read(buf[:])
			if err != nil {
				// until it ends
				if err == io.EOF {
					break
				}
				log.Fatalf("reading stderr %v", err)
			}

			if n > 0 {
				d := string(buf[:n])

				// only required if we want to see the emulator output
				log.Printf("%s", d)

				// checking for the message that it's started
				if strings.Contains(d, "Dev App Server is now running") {
					wg.Done()
				}

			}
		}
	}()

	// wait until the running message has been received
	wg.Wait()

	os.Setenv(FirestoreEmulatorHost, "localhost:8765")
	ctx := context.Background()
	users := newFirestoreTestClient(ctx).Collection("users")

	usersDataRaw, _ := os.ReadFile("../../test/data/users.json")
	var usersData []map[string]interface{}
	json.Unmarshal(usersDataRaw, &usersData)

	for _, user := range usersData {
		users.Doc(fmt.Sprintf("%v", user["id"].(float64))).Set(ctx, user)
	}

	//selectTests = append(selectTests, TestExpect{query: "select * from users", expected: usersData})
	// now it's running, we can run our unit tests
	result = m.Run()
}

func TestSelectQueries(t *testing.T) {
	for _, tt := range selectTests {
		stmt := New(&util.Context{
			ProjectId: "test",
		}, tt.query)
		actual, err := stmt.Execute()
		if err != nil {
			t.Error(err)
		} else {
			less := func(a, b string) bool { return a < b }
			if cmp.Diff(tt.columns, actual.Columns, cmpopts.SortSlices(less)) != "" {
				t.Errorf("QueryResult.Fields(%v): expected %v, actual %v", tt.query, tt.columns, actual.Columns)
			}
			if tt.length != "" && len(actual.Records) != first(strconv.Atoi(tt.length)) {
				t.Errorf("len(QueryResult.Records)(%v): expected %v, actual %v", tt.query, len(actual.Records), tt.length)
			}
			if tt.records != nil && !cmp.Equal(actual.Records, tt.records) {
				a, _ := json.Marshal(tt.records)
				log.Println(string(a))
				a, _ = json.Marshal(actual.Records)
				log.Println(string(a))
				t.Errorf("QueryResult.Records(%v): expected %v, actual %v", tt.query, tt.records, actual.Records)
			}
		}
	}
}

func first(n int, _ error) int {
	return n
}
