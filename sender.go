package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// func reader(done chan struct{}, conn *websocket.Conn) {
// 	var session string
// 	var initialized bool = false
// 	defer close(done)
// 	for {
// 		// conn.ReadJSON()
// 		_, msg, err := conn.ReadMessage()
// 		if err != nil {
// 			fmt.Println("Failed to read message:", err)
// 			return
// 		}
// 		msgs := string(msg)
// 		fmt.Println("Received websocket message:", msgs)
// 		syncId := gjson.Get(msgs, "syncId").String()
// 		if len(syncId) == 0 {
// 			if initialized {
// 				fmt.Println("Multiple init, impossible")
// 				return
// 			}
// 			fmt.Println("Special message, first handshake")
// 			r := gjson.Get(msgs, "data.code").Int()
// 			if r != 0 {
// 				fmt.Println("Handshake return is not 0, but ", r)
// 				return
// 			}
// 			session = gjson.Get(msgs, "data.session").String()
// 			fmt.Println("Connected, session", session)
// 			initialized = true
// 		} else if gjson.Get(msgs, "data.type").String() == "FriendMessage" {
// 			arr := gjson.Get(msgs, "data.messageChain").Array()
// 			for _, element := range arr {
// 				if element.Get("type").String() == "Plain" {
// 					recv := element.Get("text").String()
// 					fmt.Println("Received message:", recv)
// 					if recv == "ip" {
// 						fmt.Println("Command: IP")
// 					}
// 				}
// 			}
// 		}
// 	}
// }

// {"syncId":"","data":{"code":0,"session":"VnWy0uTZ"}}
// {"syncId":"-1","data":{"type":"FriendMessage","messageChain":[{"type":"Source","id":54303,"time":1666705058},{"type":"Plain","text":"z"}],"sender":{"id":1069350749,"nickname":"七","remark":"七"}}}
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

type Message struct {
	SyncId string
	Data   MessageData
}

func reader_json(done chan struct{}, conn *websocket.Conn) {
	var session string
	var initialized bool = false
	defer close(done)
	for {
		message := Message{}
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
			fmt.Println("Received message from 7Ji:")
			for _, msg := range message.Data.MessageChain {
				if msg.Type == "Plain" {
					fmt.Println(msg.Text)
				}
			}
		}
	}
}

// func ipCheck() {
// 	ifaces, err := net.Interfaces()
// 	if err != nil {
// 		fmt.Println("Failed to get list of interfaces", err)
// 		return
// 	}
// 	net.InterfaceByName()
// 	for _, iface := range ifaces {
// 		fmt.Println(iface.Name)
// 		addrs, err := iface.Addrs()
// 		if err != nil {
// 			fmt.Println("Failed to get address on interface", iface.Name, err)
// 		}
// 		for _, addr := range addrs {
// 			fmt.Println(addr)
// 		}

// 	}
// 	// addrs, err := net.InterfaceAddrs()
// 	// if err != nil {
// 	// 	fmt.Println("Failed to check interface addresses", err)
// 	// 	return
// 	// }
// 	// for _, addr := range addrs {
// 	// 	fmt.Println(addr)
// 	// }

// // }
//
//	func myfunc() {
//		mirai_host
//	}
type Addresses struct {
	v4_private      string
	v4_public       string
	v6_link_local   string
	v6_local_dhcp   string
	v6_local_slaac  string
	v6_global_dhcp  string
	v6_global_slaac string
}

func update_addresses(iface *net.Interface, addrs *Addresses) (update string) {
	iaddrs, err := iface.Addrs()
	if err != nil {
		fmt.Println("Failed to get address on interface", *&iface.Name)
		return
	}
	update = ""
	for _, addr := range iaddrs {
		fmt.Println(addr)
		addr_str := addr.String()
		if strings.HasPrefix(addr_str, "192.168.") {
			if addrs.v4_private != addr_str {
				update += fmt.Sprintf("v4 updated:\n%s\n", addr_str)
			}
			addrs.v4_private = addr_str
		} else if strings.HasPrefix(addr_str, "fe80:") {
			if addrs.v6_link_local != addr_str {
				update += fmt.Sprintf("v6 link-local updated:\n%s\n", addr_str)
			}
			addrs.v6_link_local = addr_str
		} else if strings.HasPrefix(addr_str, "fdb5:") {
			if strings.HasSuffix(addr_str, "/128") {
				if addrs.v6_local_dhcp != addr_str {
					update += fmt.Sprintf("v6 local dhcp updated:\n%s\n", addr_str)
				}
				addrs.v6_local_dhcp = addr_str
			} else if strings.HasSuffix(addr_str, "/64") {
				if addrs.v6_local_slaac != addr_str {
					update += fmt.Sprintf("v6 local slaac updated:\n%s\n", addr_str)
				}
				addrs.v6_local_slaac = addr_str
			}
		} else {
			if strings.HasSuffix(addr_str, "/128") {
				if addrs.v6_global_dhcp != addr_str {
					update += fmt.Sprintf("v6 global dhcp updated:\n%s\n", addr_str)
				}
				addrs.v6_global_dhcp = addr_str
			} else if strings.HasSuffix(addr_str, "/64") {
				if addrs.v6_global_slaac != addr_str {
					update += fmt.Sprintf("v6 global slaac updated:\n%s\n", addr_str)
				}
				addrs.v6_global_slaac = addr_str
			}
		}
	}
	return update
}

func main() {
	// ipCheck()
	// return
	// json
	// return
	mirai_host := flag.String("host", "localhost:8080", "hostname path to connect to")
	mirai_key := flag.String("key", "S7hpii8TFQmIZjuI9rIp", "verifyKey to be used")
	mirai_source := flag.String("source", "2821314401", "source QQ bot to be used")
	mirai_target := flag.String("target", "1069350749", "target QQ to send messeage to")
	iface_name := flag.String("iface", "eth0", "interface to get IP from")
	flag.Parse()
	iface, err := net.InterfaceByName(*iface_name)
	if err != nil {
		fmt.Println("Failed to get interface with name", *iface_name)
		return
	}
	var addrs Addresses
	update := update_addresses(iface, &addrs)
	if len(update) > 0 {
		fmt.Println(update)
	}

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
	fmt.Println("Connection successful")
	done := make(chan struct{})
	go reader_json(done, connection)
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		// case t := <-ticker.C:
		// 	fmt.Println("Periodical check at", t)
		// 	update = update_addresses(iface, &addrs)
		// 	if len(update) > 0 {
		// 		fmt.Println(update)
		// 		// connection.WriteJSON()

		// 	}
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
		fmt.Println("Wait")
	}

}
