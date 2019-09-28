package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netns"
)

type NetData struct {
	Type string
	Data [][]string
}

var AllData = map[string]string{
	"TCP":  "/proc/net/tcp",
	"UDP":  "/proc/net/udp",
	"TCP6": "/proc/net/tcp6",
	"UDP6": "/proc/net/udp6",
}

var force bool
var myPid int

func main() {
	var pids string
	flag.StringVar(&pids, "pid", "", "PID to check")
	flag.BoolVar(&force, "f", false, "Try to force scheduling on different thread.")
	flag.Parse()
	if pids == "" {
		log.Fatalf("No PIDs provided.")
	}

	myPid = syscall.Getpid()
	for _, pid := range strings.Split(pids, ",") {
		log.Infof("Analysing %s in TID: %d PID: %d", pid, syscall.Gettid(), myPid)
		p, err := strconv.Atoi(pid)
		if err != nil {
			log.Fatalf("Conversion failed: %v", err)
		}
		GetNetData(p)
	}
}

func GetNetData(pid int) {
	data := make([]NetData, len(AllData))
	var wg sync.WaitGroup
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()

		if force {
			log.Printf("Forcing schedule on a proper thread.\n")
			var tid int
			for tid = syscall.Gettid(); tid == myPid; tid = syscall.Gettid() {
				go doSthImportent()
				runtime.Gosched()
			}
		}
		tid := syscall.Gettid()
		log.Printf("We are in TID: %d\n", tid)

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		origns, err := netns.Get()
		if err != nil {
			log.Fatalf("Get origns failed: %v", err)
		}
		defer origns.Close()
		handle, err := netns.GetFromPid(pid)
		if err != nil {
			log.Fatalf("GetFromPid() failed: %v", err)
		}
		defer handle.Close()
		if err = netns.Set(handle); err != nil {
			log.Fatalf("Set ns failed: %v", err)
		}
		dataIdx := 0
		for netType, source := range AllData {
			d, err := getData(source)
			if err != nil {
				log.Fatalf("Unable to read data: %v", err)
			}
			data[dataIdx].Type = netType
			data[dataIdx].Data = d
			dataIdx++
		}
		log.Infof("Got data for %d:", pid)
		for i := range data {
			log.Infof("Got %d lines loaded for %s\n", len(data[i].Data), data[i].Type)
		}
		if pidsNs, err := os.Readlink(fmt.Sprintf("/proc/%d/ns/net", pid)); err == nil {
			log.Infof("Namespace for %d: %s", pid, pidsNs)
		}
		if pidsNs, err := os.Readlink(fmt.Sprintf("/proc/%d/task/%d/ns/net", myPid, tid)); err == nil {
			log.Infof("Namespace for tid %d: %s", tid, pidsNs)
		}

		if err = netns.Set(origns); err != nil {
			log.Fatalf("Set ns on origns failed: %v", err)
		}
	}(&wg)
	wg.Wait()
}

func doSthImportent() {
	a := 0
	for i := 0; i < 100000000; i++ {
		a += i
	}
}

func getData(file string) ([][]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := make([][]string, 0)
	l := strings.Split(string(data), "\n")
	for _, line := range l[1 : len(l)-1] {
		split := make([]string, 0)
		line = strings.TrimSpace(line)
		for _, data := range strings.Split(line, " ") {
			if data == "" || data[0] == ' ' {
				continue
			}
			split = append(split, data)
		}
		if len(split) > 0 {
			lines = append(lines, split)
		}
	}
	return lines, nil
}
