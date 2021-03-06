package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"time"

	"./multicast"
	"golang.org/x/net/ipv4"
)

const (
	usage = `mping version: mping/1.2
Usage: ./mping [-h] [-s sendGroup] [-r receiveGroup] [-l localAddress] [-S sourceAddress] [-m message] [-i interval] [-log path]

Options:
`
)

const (
	MAX_DATA_SIZE = 65504
	FIT_DATA_SIZE = 1472
)

var (
	help     bool
	test     bool
	realtime bool
	hexdata  bool
	logPath  string

	sendAddress    string
	receiveAddress string
	localAddress   string
	sourceAddress  string
	content        string
	content_byte   []byte
	interval       int
	dataSize       int

	clock_start    time.Time
	clock_end      time.Time
	clock_mutex    bool
	bytes_send_sum float32
	bytes_rev_sum  float32

	rawlog *log.Logger

	ipReg   *regexp.Regexp
	addrReg *regexp.Regexp
)

func init() {
	clock_mutex = false
	bytes_send_sum = 0
	bytes_rev_sum = 0
	ipReg, _ = regexp.Compile(`((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})(\.((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})){3}`)
	addrReg, _ = regexp.Compile(`((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})(\.((2(5[0-5]|[0-4]\d))|[0-1]?\d{1,2})){3}:(([2-9]\d{3})|([1-5]\d{4})|(6[0-4]\d{3})|(65[0-4]\d{2})|(655[0-2]\d)|(6553[0-5]))`)
	flagSettup()
}

func main() {
	flag.Parse()
	logSettup()
	processArgs()
	processCommands()
}

func msgReceiveHandler(cm *ipv4.ControlMessage, src net.Addr, n int, b []byte) {
	if cm != nil {
		log.Println(cm.String())
	}
	if clock_mutex == false {
		clock_start = time.Now()
		clock_mutex = true
	} else {
		clock_end = time.Now()
		bytes_rev_sum = bytes_rev_sum + float32(n)
		rates_rev := bytes_rev_sum * 1000000000 / float32(clock_end.Sub(clock_start).Nanoseconds())
		if rates_rev < 1000 {
			log.Println(rates_rev, "Bps")
		} else if rates_rev < 1000000 {
			log.Println(rates_rev/1024, "KBps")
		} else if rates_rev < 1000000000 {
			log.Println(rates_rev/1024/1024, "MBps")
		}
	}
	log.Println(n, "bytes read from", src)
	if hexdata {
		rawlog.Println(hex.Dump(b[:n]))
	}
}

func msgSendHandler(n int, b []byte) {
	if clock_mutex == false {
		clock_start = time.Now()
		clock_mutex = true
	} else {
		clock_end = time.Now()
		bytes_send_sum = bytes_send_sum + float32(n)
		rates_send := bytes_send_sum * 1000000000 / float32(clock_end.Sub(clock_start).Nanoseconds())
		if rates_send < 1000 {
			log.Println(rates_send, "Bps")
		} else if rates_send < 1000000 {
			log.Println(rates_send/1024, "KBps")
		} else if rates_send < 1000000000 {
			log.Println(rates_send/1024/1024, "MBps")
		}
	}
	log.Println(n, "bytes has been sent")
	if hexdata {
		rawlog.Println(hex.Dump(b[:n]))
	}
}

func getifi(addr string) (*net.Interface, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if host == "127.0.0.1" {
		return nil, nil
	}
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(netInterfaces); i++ {
		if (netInterfaces[i].Flags & net.FlagUp) != 0 {
			addrs, _ := netInterfaces[i].Addrs()
			for _, address := range addrs {
				ipv4 := ipReg.FindString(address.String())
				if ipv4 == host {
					ifi := &netInterfaces[i]
					// index := netInterfaces[i].Index
					// ifi, err := net.InterfaceByIndex(index)
					// if err != nil {
					// 	return nil, err
					// }
					return ifi, nil
				}
			}
		}
	}
	return nil, nil
}

func logSettup() {
	// set the formatflag of log
	// log.SetFlags(log.Lshortfile | log.LstdFlags)
	log.SetFlags(log.LstdFlags)
	// define the log file
	if logPath != "/" {
		file := logPath + time.Now().Format("2006-01-02 15-04") + ".log"
		logFile, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
		if err != nil {
			log.Fatal(err)
		}
		writers := []io.Writer{
			logFile,
			os.Stdout,
		}
		fileAndStdoutWriter := io.MultiWriter(writers...)
		log.SetOutput(fileAndStdoutWriter)
		rawlog = log.New(fileAndStdoutWriter, "", 0)
	} else {
		rawlog = log.New(os.Stdout, "", 0)
	}
}

func flagSettup() {
	flag.BoolVar(&help, "h", false, "this help")
	flag.BoolVar(&test, "test", false, "send and receive locally to examinate a test")
	flag.BoolVar(&realtime, "time", false, "send real time as the content to examinate")
	flag.BoolVar(&hexdata, "x", false, "whether to show the hex data")
	flag.StringVar(&logPath, "log", "/", "[/tmp/] or [C:\\] determine whether to log, Path e.g ./, Forbidden /")
	flag.StringVar(&sendAddress, "s", "239.255.255.255:9999", "[group:port] send packet to group")
	flag.StringVar(&receiveAddress, "r", "239.255.255.255:9999", "[group:port] receive packet from group")
	flag.StringVar(&localAddress, "l", "127.0.0.1:8888", "[ip[:port]] must choose your local using interface")
	flag.StringVar(&sourceAddress, "S", "127.0.0.1:8888", "[ip[:port]] must determine the peer source ip if using SSM")
	flag.StringVar(&content, "m", "hello, world\n", "[[]byte] change the content of sending")
	flag.IntVar(&interval, "i", 1000000000, "[number] change the interval between package sent (unit:Nanosecond)")
	flag.IntVar(&dataSize, "p", -1, "[number] the size of payload data(0 means use 1472 bytes payloads)")
	flag.Usage = flagUsage
}

func flagUsage() {
	fmt.Fprintf(os.Stderr, usage)
	flag.PrintDefaults()
}

func processCommands() {
	if help {
		flag.Usage()
		return
	}
	// determine the selected interface
	ifi, err := getifi(localAddress)
	if ifi != nil {
		log.Println("The index of interface used is", ifi.Index+1)
		log.Println("The name of interface used is", ifi.Name)
	} else {
		fmt.Println("[Tips:determine your using interface IP]")
		fmt.Println("[Otherwise the result may be incorrect]")
	}
	if err != nil {
		log.Fatal(err)
	}
	if test {
		content_byte = []byte(content)
		go multicast.Send(sendAddress, localAddress, content_byte, interval, msgSendHandler)
		err = multicast.Receive(receiveAddress, sourceAddress, ifi, msgReceiveHandler)
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	if realtime {
		content = time.Now().Format("2006-01-02 15:04:05")
		content_byte = []byte(content)
	}
	if dataSize == -1 {
		content_byte = []byte(content)
	} else if dataSize == 0 {
		dataSize = FIT_DATA_SIZE
		var data []byte = make([]byte, dataSize-len(content))
		content_byte = strconv.AppendQuoteToASCII(data, content)
	} else if dataSize > len(content) && dataSize <= MAX_DATA_SIZE {
		var data []byte = make([]byte, dataSize-len(content))
		content_byte = strconv.AppendQuoteToASCII(data, content)
	} else if dataSize > MAX_DATA_SIZE {
		log.Fatal("big packet")
	}
	if (sendAddress != "239.255.255.255:9999") && (receiveAddress != "239.255.255.255:9999") {
		log.Println("Send to ", sendAddress)
		go multicast.Send(sendAddress, localAddress, content_byte, interval, msgSendHandler)
		log.Println("Receive from ", receiveAddress)
		err := multicast.Receive(receiveAddress, sourceAddress, ifi, msgReceiveHandler)
		if err != nil {
			log.Fatal(err)
		}
	} else if sendAddress != "239.255.255.255:9999" && (receiveAddress == "239.255.255.255:9999") {
		log.Println("Send to ", sendAddress)
		err := multicast.Send(sendAddress, localAddress, content_byte, interval, msgSendHandler)
		if err != nil {
			log.Fatal(err)
		}
	} else if receiveAddress != "239.255.255.255:9999" && (sendAddress == "239.255.255.255:9999") {
		log.Println("Receive from ", receiveAddress)
		err := multicast.Receive(receiveAddress, sourceAddress, ifi, msgReceiveHandler)
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println(`Please input the right arguments(use "-h" to see help)`)
}

func processArgs() {
	if !addrReg.MatchString(localAddress) {
		conn, err := net.ListenUDP("udp", nil)
		if err != nil {
			log.Fatal(err)
		}
		port := conn.LocalAddr().(*net.UDPAddr).Port
		localAddress = net.JoinHostPort(localAddress, strconv.Itoa(port))
		conn.Close()
	}
	if !addrReg.MatchString(sourceAddress) {
		sourceAddress = net.JoinHostPort(sourceAddress, "0")
	}
}
