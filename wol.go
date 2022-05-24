/**
go-email-wol
@Author:BH6AOL <bh6aol@gmail.com>
@Date:2022-04-10 v0.0.1

编译命令:
SET GOOS=linux
SET GOARCH=mipsle
go build wol.go

后台运行
nohup ./wol 2>&1 &
 */
package main

import (
	"bytes"
	"encoding/hex"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"gopkg.in/ini.v1"
	"log"
	"net"
	"strings"
	"time"
)

var cfg *ini.File

func BytesCombine(pBytes ...[]byte) []byte {
	return bytes.Join(pBytes, []byte(""))
}

func CheckInBox() ([]byte,bool) {

	username := cfg.Section("email").Key("username").String()
	password := cfg.Section("email").Key("password").String()
	controlMail := cfg.Section("email").Key("controlMail").String()
	imapServer := cfg.Section("email").Key("imapServer").String()

	// Connect to server
	c, err := client.DialTLS(imapServer, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Don't forget to logout
	defer c.Logout()

	// Login
	if err := c.Login(username, password); err != nil {
		log.Fatal(err)
	}

	// List mailboxes
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)

	done <- c.List("", "*", mailboxes)

	//log.Println("Mailboxes:")
	//for m := range mailboxes {
	//	log.Println("* " + m.Name)
	//}

	if err := <-done; err != nil {
		log.Fatal(err)
	}

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		log.Fatal(err)
	}
	//log.Println("Flags for INBOX:", mbox.Flags)

	// Get the last 4 messages
	from := uint32(1)
	to := mbox.Messages
	if mbox.Messages > 3 {
		// We're using unsigned integers here, only subtract if the result is > 0
		from = mbox.Messages - 3
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, 10)
	done = make(chan error, 1)

	done <- c.Fetch(seqset, []imap.FetchItem{imap.FetchEnvelope,imap.FetchFlags}, messages)

	//log.Println("Last 4 messages:")


	var mac []byte
	var needWOL bool
	for msg := range messages {
		// 只检查来自控制端的未读邮件
		//log.Println(msg.Envelope.Subject,msg.Envelope.From[0].Address(),msg.Flags)
		if msg.Envelope.From[0].Address() == controlMail && len(msg.Flags) == 0{
			log.Println("find boot email: ",msg.Envelope.Subject,msg.Envelope.From[0].Address())
			// Here you tell to add the flag \Seen
			seqSet := new(imap.SeqSet)
			seqSet.AddNum(msg.SeqNum)
			item := imap.FormatFlagsOp(imap.AddFlags, true)
			flags := []interface{}{imap.SeenFlag}
			err = c.Store(seqSet, item, flags, nil)
			if err != nil {
				log.Fatal(err)
			}

			needWOL = true
			s := strings.Replace(msg.Envelope.Subject, "-","",-1)
			mac, err = hex.DecodeString(s)
			if err != nil {
				log.Fatal(err)
			}
			break
		}
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}
	return mac,needWOL
}

func BootByMac(mac []byte) {

	broadcastStr := cfg.Section("wol").Key("broadcast").String()
	port,err := cfg.Section("wol").Key("port").Int()
	if err != nil {
		log.Fatal("error port number:", err)
	}
	broadcastIp := net.ParseIP(broadcastStr)
	if err != nil {
		log.Fatal("error broadcast:", err)
	}

	socket, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:  broadcastIp,
		Port: port,
	})
	if err != nil {
		log.Fatal("udp error:", err)
	}
	defer func(socket *net.UDPConn) {
		err := socket.Close()
		if err != nil {
			log.Println("udp close error:", err)
		}
	}(socket)

	ff6 := []byte{0xFF,0xFF,0xFF,0xFF,0xFF,0xFF}
	var mac16 []byte
	for i := 0; i < 16; i++ {
		mac16 = BytesCombine(mac16,mac)
	}
	sendData := BytesCombine(ff6,mac16)
	//fmt.Println(sendData)
	_, err = socket.Write(sendData)
	if err != nil {
		log.Fatal("data send error:", err)
	}else{
		log.Println("data send success")
	}
}


func main() {

	var err error
	cfg, err = ini.Load("wol.ini")
	if err != nil {
		log.Fatal("fail to read file:", err)
	}

	for{
		mac, needBoot := CheckInBox()
		if needBoot {
			BootByMac(mac)
		}
		checkInterval, err := cfg.Section("wol").Key("checkInterval").Int()
		if err != nil {
			checkInterval = 300
		}
		time.Sleep(time.Duration(checkInterval) * time.Second)
	}

}
