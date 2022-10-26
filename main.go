package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type MessageChainEntry struct {
	Type string
	Id   int
	Time int
	Text string
}

type MessageSender struct {
	Id       int
	Nickname string
	Remark   string
}

type MessageData struct {
	Code         int
	Session      string
	Type         string
	MessageChain []MessageChainEntry
	Sender       MessageSender
}

type MessageReceive struct {
	SyncId string
	Data   MessageData
}

type MessageChainEntryPlain struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type MessageContent struct {
	Target       int64                    `json:"target"`
	MessageChain []MessageChainEntryPlain `json:"messageChain"`
}

type MessageSend struct {
	SyncId     int            `json:"syncId"`
	Command    string         `json:"command"`
	SubCommand string         `json:"subCommand"`
	Content    MessageContent `json:"content"`
}

func reader_json(done chan struct{}, conn *websocket.Conn, addrs *Addresses, target int64) {
	var session string
	var initialized bool = false
	defer close(done)
	for {
		message := MessageReceive{}
		err := conn.ReadJSON(&message)
		if err != nil {
			fmt.Println("Failed to read message JSON", err)
			return
		}
		if message.SyncId == "" {
			fmt.Println("Special message, first handshake")
			if initialized {
				fmt.Println("Error: double initialization")
				return
			}
			initialized = true
			session = message.Data.Session
			fmt.Println("Handshake successful, session ID", session)
		} else if message.Data.Type == "FriendMessage" && message.Data.Sender.Id == 1069350749 {
			for _, msg := range message.Data.MessageChain {
				if msg.Type == "Plain" {
					fmt.Println("Received message from 7Ji:", msg.Text)
					if msg.Text == "ip" {
						fmt.Println("Responsing to command ip")
						report := fmt.Sprintf("IPv4 public:\n%s\n\nIPv4 private:\n%s\n\nIPv6 link local:\n%s\n\nIPv6 local DHCP:\n%s\n\nIPv6 local SLAAC:\n%s\n\nIPv6 global DHCP:\n%s\n\nIPv6 global SLAAC:\n%s", addrs.v4_public_router, addrs.v4_private, addrs.v6_link_local, addrs.v6_local_dhcp, addrs.v6_local_slaac, addrs.v6_global_dhcp, addrs.v6_global_slaac)
						err = conn.WriteJSON(send_helper(target, report))
						if err != nil {
							fmt.Println("Failed to response to command ip")
							return
						}
					}
				}
			}
		}
	}
}

type Addresses struct {
	v4_private       string
	v4_public_router string
	v4_public_report string
	v6_link_local    string
	v6_local_dhcp    string
	v6_local_slaac   string
	v6_global_dhcp   string
	v6_global_slaac  string
}

func listener_process(c *net.Conn, addrs *Addresses) {
	defer (*c).Close()
	for {
		netData, err := bufio.NewReader(*c).ReadString('\n')
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("Router message ends")
			} else {
				fmt.Println("Failed to process router message", err)
			}
			return
		}
		if strings.TrimSpace(string(netData)) == "STOP" {
			fmt.Println("Exiting TCP server!")
			return
		}
		// fmt.Print("-> ", string(netData))
		addrs.v4_public_router = strings.TrimRight(string(netData), "\n")
		t := time.Now()
		myTime := t.Format(time.RFC3339) + "\n"
		(*c).Write([]byte(myTime))
	}
}

func listener(l *net.Listener, addrs *Addresses) {
	// defer (*l).Close()
	for {
		c, err := (*l).Accept()
		if err != nil {
			fmt.Println("Failed to accept TCP incoming stream, ", err)
			return
		}
		go listener_process(&c, addrs)
	}
}

func update_addresses(iface *net.Interface, addrs *Addresses) (report string) {
	iaddrs, err := iface.Addrs()
	if err != nil {
		fmt.Println("Failed to get address on interface", *&iface.Name)
		return
	}
	report = ""
	start := false
	if addrs.v4_public_router != addrs.v4_public_report {
		report += fmt.Sprintf("v4 public updated:\n%s", addrs.v4_public_router)
		addrs.v4_public_report = addrs.v4_public_router
		start = true
	}
	for _, addr := range iaddrs {
		// fmt.Println(addr)
		addr_str := addr.String()
		if strings.HasPrefix(addr_str, "192.168.") {
			if addrs.v4_private != addr_str {
				if start {
					report += "\n\n"
				}
				report += fmt.Sprintf("v4 private updated:\n%s", addr_str)
				addrs.v4_private = addr_str
				start = true
			}
		} else if strings.HasPrefix(addr_str, "fe80:") {
			if addrs.v6_link_local != addr_str {
				if start {
					report += "\n\n"
				}
				report += fmt.Sprintf("v6 link-local updated:\n%s", addr_str)
				addrs.v6_link_local = addr_str
				start = true
			}
		} else if strings.HasPrefix(addr_str, "fdb5:") {
			if strings.HasSuffix(addr_str, "/128") {
				if addrs.v6_local_dhcp != addr_str {
					if start {
						report += "\n\n"
					}
					report += fmt.Sprintf("v6 local dhcp updated:\n%s", addr_str)
					addrs.v6_local_dhcp = addr_str
					start = true
				}
			} else if strings.HasSuffix(addr_str, "/64") {
				if addrs.v6_local_slaac != addr_str {
					if start {
						report += "\n\n"
					}
					report += fmt.Sprintf("v6 local slaac updated:\n%s", addr_str)
					addrs.v6_local_slaac = addr_str
					start = true
				}
			}
		} else {
			if strings.HasSuffix(addr_str, "/128") {
				if addrs.v6_global_dhcp != addr_str {
					if start {
						report += "\n\n"
					}
					report += fmt.Sprintf("v6 global dhcp updated:\n%s", addr_str)
					addrs.v6_global_dhcp = addr_str
					start = true
				}
			} else if strings.HasSuffix(addr_str, "/64") {
				if addrs.v6_global_slaac != addr_str {
					if start {
						report += "\n\n"
					}
					report += fmt.Sprintf("v6 global slaac updated:\n%s", addr_str)
					addrs.v6_global_slaac = addr_str
					start = true
				}
			}
		}
	}
	return report
}

func send_helper(target int64, msg string) MessageSend {
	msg_s := MessageSend{
		SyncId:  123,
		Command: "sendFriendMessage",
		Content: MessageContent{
			Target: target,
			MessageChain: []MessageChainEntryPlain{
				{
					Type: "Plain",
					Text: msg,
				},
			},
		},
	}
	return msg_s
}

func main() {
	mirai_host := flag.String("host", "localhost:8080", "hostname path to connect to")
	mirai_key := flag.String("key", "S7hpii8TFQmIZjuI9rIp", "verifyKey to be used")
	mirai_source := flag.String("source", "2821314401", "source QQ bot to be used")
	mirai_target := flag.String("target", "1069350749", "target QQ to send messeage to")
	listen := flag.String("listen", ":7777", "[host:]port to listen router report on")
	iface_name := flag.String("iface", "eth0", "interface to get IP from")
	flag.Parse()
	target, _ := strconv.ParseInt(*mirai_target, 10, 64)
	iface, err := net.InterfaceByName(*iface_name)
	if err != nil {
		fmt.Println("Failed to get interface with name", *iface_name)
		return
	}
	listener_router, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Println("Failed to listen router IP report", err)
		return
	}
	defer listener_router.Close()
	url := url.URL{Scheme: "ws", Host: *mirai_host, Path: "/message"}
	url_string := url.String()
	fmt.Printf("Connecting to mirai on WS path '%s', with key '%s', source QQ '%s', target QQ '%s'\n", url_string, *mirai_key, *mirai_source, *mirai_target)
	header := make(http.Header)
	header.Add("verifyKey", *mirai_key)
	header.Add("qq", *mirai_source)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	connection, _, err := websocket.DefaultDialer.Dial(url_string, header)
	if err != nil {
		fmt.Println("Failed to connect, quiting")
		return
	}
	defer connection.Close()
	var addrs Addresses
	update := update_addresses(iface, &addrs)
	go listener(&listener_router, &addrs)
	if len(update) > 0 {
		msg := send_helper(target, update)
		err = connection.WriteJSON(msg)
		if err != nil {
			fmt.Println("Failed to write test message", err)
			return
		}
	}
	fmt.Println("Connection successful")
	done := make(chan struct{})
	go reader_json(done, connection, &addrs, target)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
			// case t := <-ticker.C:
		case <-ticker.C:
			// fmt.Println("Periodical check at", t)
			// fmt.Println("Periodical check")
			update = update_addresses(iface, &addrs)
			if len(update) > 0 {
				msg := send_helper(target, update)
				err = connection.WriteJSON(msg)
				if err != nil {
					fmt.Println("Failed to write test message", err)
					return
				}
			}
		case <-interrupt:
			log.Println("interrupt received, wait for others")
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			fmt.Println("Ending...")
			return
			// case <-time.After(time.Second):
			// 	fmt.Println("Dumb wait 1s")
		}
		// fmt.Println("Wait")
	}

}
