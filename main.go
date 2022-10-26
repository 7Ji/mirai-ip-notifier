package main

import (
	"bufio"
	"encoding/json"
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
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

/*  Handshake
{
	"syncId": "",
	"data": {
		"code": 0,
		"session": "1BrU0vUN"
	}
}
*/

/*  FriendMessage
{
	"syncId": "-1",
	"data": {
		"type": "FriendMessage",
		"messageChain":[
			{
				"type":"Source",
				"id":56989,
				"time":1666766438
			},
			{
				"type":"Plain",
				"text":"ip"
			}
		],
		"sender":{
			"id": 1069350749,
			"nickname": "七",
			"remark": "七"
		}
	}
} */

/* MessageConfirm success
{
	"syncId": "123",
	"data": {
		"code": 0,
		"msg": "success",
		"messageId":12800
	}
} */

/* MessageConfirm fail
{
	"syncId": "123",
	"data": {
		"code": 5,
		"msg": "指定对象不存在"
	}
} */

/* BotOffline (drop)
{
	"syncId": "-1",
	"data": {
		"type": "BotOfflineEventDropped",
		"qq": 2821314401
	}
} */

/* BotOnline
{
	"syncId": "-1",
	"data": {
		"type": "BotOnlineEvent",
		"qq": 2821314401
	}
} */

/* BotRelogin
{
	"syncId": "-1",
	"data": {
		"type": "BotReloginEvent",
		"qq": 2821314401
	}
} */

/* BotOffline (exit)
{
	"syncId": "-1",
	"data": {
		"type": "BotOfflineEventActive",
		"qq": 2821314401
	}
} */

type MessageChainEntry struct {
	Type string
	Id   int
	Time int
	Text string
}

type MessageSender struct {
	Id       int64
	Nickname string
	Remark   string
}

type MessageData struct {
	Code         int
	Msg          string
	MessageId    int
	Session      string
	Type         string
	QQ           int64
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

type MiraiConf struct {
	SourceString string
	SourceLong   int64
	SourceUlong  uint64
	TargetString string
	TargetLong   int64
	TargetUlong  uint64
	Session      string
}

func reader(done chan struct{}, conn *websocket.Conn, addrs *Addresses, mconf *MiraiConf, syncIdHighest *uint, queue map[uint](chan bool), mx_id *sync.Mutex, mx_pending *sync.Mutex, online *bool) {
	defer close(done)
	for {
		_, msg_bytes, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Failed to read message", err)
		}
		fmt.Println("Received message", string(msg_bytes))
		message := MessageReceive{}
		err = json.Unmarshal(msg_bytes, &message)
		if err != nil {
			fmt.Println("Failed to read message into JSON", err)
			return
		}
		if message.SyncId == "" {
			fmt.Println("Error: double initialization")
			return
		} else if message.SyncId == "-1" {
			fmt.Println("Received active message sent by server, type", message.Data.Type)
			switch message.Data.Type {
			case "FriendMessage":
				if message.Data.Sender.Id == mconf.TargetLong {
					fmt.Println("Process message sent by", mconf.TargetLong)
					for _, msg := range message.Data.MessageChain {
						if msg.Type == "Plain" {
							fmt.Println("Received message from 7Ji:", msg.Text)
							switch msg.Text {
							case "ip":
								fmt.Println("Responsing to command ip (ip summary)")
								report := fmt.Sprintf("IPv4 public:\n%s\n\nIPv4 private:\n%s\n\nIPv6 link local:\n%s\n\nIPv6 local DHCP:\n%s\n\nIPv6 local SLAAC:\n%s\n\nIPv6 global DHCP:\n%s\n\nIPv6 global SLAAC:\n%s", addrs.v4_public_router, addrs.v4_private, addrs.v6_link_local, addrs.v6_local_dhcp, addrs.v6_local_slaac, addrs.v6_global_dhcp, addrs.v6_global_slaac)
								go send_message(conn, mconf, report, syncIdHighest, queue, mx_id, mx_pending, online)
							case "hi":
								fmt.Println("Responsing to command hi (say hi)")
								go send_message(conn, mconf, "hello", syncIdHighest, queue, mx_id, mx_pending, online)
							default:
								go send_message(conn, mconf, "only ip/hi are supported", syncIdHighest, queue, mx_id, mx_pending, online)
							}
						}
					}
				} else {
					fmt.Println("Ignore message sent by", message.Data.Sender.Id)
				}
			case "BotOfflineEventDropped":
				fmt.Println("Bot offline for drop, waiting for online")
				*online = false
			case "BotOnlineEvent":
				fmt.Println("Bot online again")
				*online = true
			case "BotReloginEvent":
				fmt.Println("Bot relogin")
				*online = true
			case "BotOfflineEventActive":
				fmt.Println("Bot offline active, existing")
				return
			default:
				fmt.Println("Message/Event type not implemented yet:", message.Data.Type)
			}
		} else {
			fmt.Println("Received passive message for confimation, syncId", message.SyncId)
			syncId_uint, err := strconv.ParseUint(message.SyncId, 10, 32)
			if err != nil {
				fmt.Println("Failed to convert syncId to uint")
				return
			}
			msg_sync, ok := queue[uint(syncId_uint)]
			if ok {
				if message.Data.Code == 0 {
					fmt.Println("Confirmed message sent success", message.SyncId)
					msg_sync <- true
				} else {
					fmt.Println("Confirmed message failed to send", message.SyncId)
					msg_sync <- false
				}
			} else {
				// Maybe this can be ignored?
				fmt.Println("SyncID is not in queue yet mirai sent its completion, ignore that", message.SyncId)
				// return
			}
		}
	}
}

// func request_list(conn *websocket.Conn) {
// 	for {
// 		msg := `{"syncId":233,"command":"botList","subCommand":null,"content":{}}`
// 		conn.WriteMessage(websocket.TextMessage, []byte(msg))
// 		time.Sleep(time.Second)
// 	}
// }

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

// func send_helper(target int64, msg string) MessageSend {
// 	msg_s := MessageSend{
// 		SyncId:  123,
// 		Command: "sendFriendMessage",
// 		Content: MessageContent{
// 			Target: target,
// 			MessageChain: []MessageChainEntryPlain{
// 				{
// 					Type: "Plain",
// 					Text: msg,
// 				},
// 			},
// 		},
// 	}
// 	return msg_s
// }

func send_message(conn *websocket.Conn, mconf *MiraiConf, text string, syncIdHighest *uint, queue map[uint](chan bool), mx_id *sync.Mutex, mx_pending *sync.Mutex, online *bool) {
	for {
		mx_id.Lock()
		syncId := *syncIdHighest
		*syncIdHighest++
		mx_id.Unlock()
		fmt.Println("Sending message", syncId)
		for !(*online) {
			fmt.Println("Waiting for online, message halted", syncId)
			time.Sleep(time.Second)
		}
		// syncIdString := fmt.Sprintln(syncIdInt)
		msg := MessageSend{
			SyncId:  int(syncId),
			Command: "sendFriendMessage",
			Content: MessageContent{
				Target: mconf.TargetLong,
				MessageChain: []MessageChainEntryPlain{
					{
						Type: "Plain",
						Text: text,
					},
				},
			},
		}
		err := conn.WriteJSON(&msg)
		if err != err {
			fmt.Println("Failed to send message", syncId, err)
		}
		mx_pending.Lock()
		queue[syncId] = make(chan bool)
		mx_pending.Unlock()
		r := <-queue[syncId]
		close(queue[syncId])
		mx_pending.Lock()
		delete(queue, syncId)
		mx_pending.Unlock()
		if r {
			fmt.Println("Message sent successfully", syncId)
			return
		} else {
			fmt.Println("Failed to send message, retrying", syncId)
		}
	}
}

func main() {
	flag_host := flag.String("host", "localhost:8080", "hostname path to connect to")
	flag_key := flag.String("key", "S7hpii8TFQmIZjuI9rIp", "verifyKey to be used")
	flag_source := flag.Uint64("source", 2821314401, "source QQ bot to be used")
	flag_target := flag.Uint64("target", 1069350749, "target QQ to send messeage to")
	flag_listen := flag.String("listen", ":7777", "[host:]port to listen router report on")
	flag_iface := flag.String("iface", "eth0", "interface to get IP from")
	flag.Parse()
	mconf := MiraiConf{
		SourceString: fmt.Sprintln(*flag_source),
		SourceLong:   int64(*flag_source),
		SourceUlong:  *flag_source,
		TargetString: fmt.Sprintln(*flag_target),
		TargetLong:   int64(*flag_target),
		TargetUlong:  *flag_target,
	}
	iface, err := net.InterfaceByName(*flag_iface)
	if err != nil {
		fmt.Println("Failed to get interface with name", *flag_iface)
		return
	}
	listener_router, err := net.Listen("tcp", *flag_listen)
	if err != nil {
		fmt.Println("Failed to listen router IP report", err)
		return
	}
	defer listener_router.Close()
	url := url.URL{Scheme: "ws", Host: *flag_host, Path: "/all"}
	url_string := url.String()
	fmt.Println("Connecting to mirai at", url_string)
	// fmt.Printf("Connecting to mirai on WS path '%s', with key '%s', source QQ '%s', target QQ '%s'\n", url_string, *flag_key, *flag_source, *flag_target)
	header := http.Header{}
	header.Add("verifyKey", *flag_key)
	header.Add("qq", mconf.SourceString)
	var connection *websocket.Conn
	for i := 1; i < 4; i++ {
		connection, _, err = websocket.DefaultDialer.Dial(url_string, header)
		if err == nil {
			break
		} else {
			fmt.Println("Failed to connect to mirai on try", i, "of", 3, err)
			time.Sleep(time.Second * 5)
		}
	}
	if err != nil {
		fmt.Println("Failed to connect to mirai after 3 times, give up", err)
	}
	defer connection.Close()
	_, msg, err := connection.ReadMessage()
	fmt.Println("Handshake message:", string(msg))
	msg_recv := MessageReceive{}
	err = json.Unmarshal(msg, &msg_recv)
	if err != nil {
		fmt.Println("Failed to read session message, quiting")
		return
	}
	if msg_recv.SyncId == "" && msg_recv.Data.Code == 0 {
		mconf.Session = msg_recv.Data.Session
	} else {
		fmt.Println("Failed to get session, quiting")
		return
	}
	fmt.Println("Connection successful")
	addrs := Addresses{}
	// update := update_addresses(iface, &addrs)
	go listener(&listener_router, &addrs)
	// if len(update) > 0 {
	// 	msg := send_helper(target, update)
	// 	err = connection.WriteJSON(msg)
	// 	if err != nil {
	// 		fmt.Println("Failed to write test message", err)
	// 		return
	// 	}
	// }
	var sync_id_highest uint = 100
	var mx_id, mx_pending sync.Mutex
	queue := make(map[uint](chan bool))
	done := make(chan struct{})
	online := true
	go reader(done, connection, &addrs, &mconf, &sync_id_highest, queue, &mx_id, &mx_pending, &online)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	// go request_list(connection)
	for {
		select {
		case <-done:
			return
			// case t := <-ticker.C:
		case <-ticker.C:
			// fmt.Println("Periodical check at", t)
			// fmt.Println("Periodical check")
			update := update_addresses(iface, &addrs)
			if len(update) > 0 {
				go send_message(connection, &mconf, update, &sync_id_highest, queue, &mx_id, &mx_pending, &online)
				// msg := send_helper(target, update)
				// err = connection.WriteJSON(msg)
				// if err != nil {
				// 	fmt.Println("Failed to write test message", err)
				// 	return
				// }
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
