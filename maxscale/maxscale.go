// maxscale.go

package maxscale

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type MaxScale struct {
	Host string
	Port string
	User string
	Pass string
	Conn net.Conn
}

type Server struct {
	Server  string
	Address string
}

type ServerMaxinfo struct {
	Server      string
	Address     string
	Port        int
	Connections int
	Status      string
}

type ServerList []Server

var ServerMaxinfos = make([]ServerMaxinfo, 0)

const (
	maxDefaultPort    = "6603"
	maxDefaultUser    = "admin"
	maxDefaultPass    = "mariadb"
	maxDefaultTimeout = (10 * time.Second)
	// Error types
	ErrorNegotiation = "Incorrect maxscale protocol negotiation"
	ErrorReader      = "Error reading from buffer"
)

func (m *MaxScale) Connect() error {
	var err error
	address := fmt.Sprintf("%s:%s", m.Host, m.Port)
	m.Conn, err = net.DialTimeout("tcp", address, maxDefaultTimeout)
	if err != nil {
		return errors.New(fmt.Sprintf("Connection failed to address %s", address))
	}
	reader := bufio.NewReader(m.Conn)
	buf := make([]byte, 80)
	res, err := reader.Read(buf)
	if err != nil {
		return errors.New(ErrorReader)
	}
	if res != 4 {
		return errors.New(ErrorNegotiation)
	}
	writer := bufio.NewWriter(m.Conn)
	fmt.Fprint(writer, m.User)
	writer.Flush()
	res, err = reader.Read(buf)
	if err != nil {
		return errors.New(ErrorReader)
	}
	if res != 8 {
		return errors.New(ErrorNegotiation)
	}
	fmt.Fprint(writer, m.Pass)
	writer.Flush()
	res, err = reader.Read(buf)
	if err != nil {
		return errors.New(ErrorReader)
	}
	if string(buf[0:6]) == "FAILED" {
		return errors.New("Authentication failed")
	}
	return nil
}

func (m *MaxScale) GetMaxInfoServers(url string) ([]ServerMaxinfo, error) {
	client := &http.Client{}

	// Send the request via a client
	// Do sends an HTTP request and
	// returns an HTTP response
	// Build the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal("NewRequest: ", err)
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal("Do: ", err)
		return nil, err
	}

	// Callers should close resp.Body
	// when done reading from it
	// Defer the closing of the body
	defer resp.Body.Close()
	monjson, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Do: ", err)
		return nil, err
	}

	// Use json.Decode for reading streams of JSON data
	if err := json.Unmarshal(monjson, &ServerMaxinfos); err != nil {
		log.Println(err)
	}
	return ServerMaxinfos, nil
}

func (m *MaxScale) ShowServers() ([]byte, error) {
	m.Command("show serversjson")
	reader := bufio.NewReader(m.Conn)
	var response []byte
	buf := make([]byte, 80)
	for {
		res, err := reader.Read(buf)
		if err != nil {
		}
		str := string(buf[0:res])
		if res < 80 && strings.HasSuffix(str, "OK") {
			response = append(response, buf[0:res-2]...)
			break
		}
		response = append(response, buf[0:res]...)
	}
	return response, nil
}

func (m *MaxScale) ListServers() (ServerList, error) {
	m.Command("list servers")
	reader := bufio.NewReader(m.Conn)
	var response []byte
	buf := make([]byte, 80)
	for {
		res, err := reader.Read(buf)
		if err != nil {
		}
		str := string(buf[0:res])
		if res < 80 && strings.HasSuffix(str, "OK") {
			response = append(response, buf[0:res-2]...)
			break
		}
		response = append(response, buf[0:res]...)
	}
	list := strings.Split(string(response), "\n")
	var sl ServerList
	for _, line := range list {
		re := regexp.MustCompile(`^([0-9A-Za-z]+)[[:space:]]*\|[[:space:]]*([0-9A-Za-z]+)[[:space:]]*\|[[:space:]]*`)
		match := re.FindStringSubmatch(line)
		if len(match) > 0 {
			if match[0] != "" && match[1] != "Server" {
				item := Server{Server: match[1], Address: match[2]}
				sl = append(sl, item)
			}
		}
	}
	return sl, nil
}

func (sl ServerList) GetServer(ip string) string {
	for _, s := range sl {
		if s.Address == ip {
			return s.Server
		}
	}
	return ""
}

func (m *MaxScale) GetMaxInfoServer(ip string, port int) (string, string, int) {
	for _, s := range ServerMaxinfos {
		//	log.Printf("%s,%s", s.Address, ip)
		if s.Address == ip && s.Port == port {
			return s.Server, s.Status, s.Connections
		}
	}
	return "", "", 0
}

func (m *MaxScale) Command(cmd string) error {
	writer := bufio.NewWriter(m.Conn)
	if _, err := fmt.Fprint(writer, cmd); err != nil {
		return err
	}
	err := writer.Flush()
	return err
}
