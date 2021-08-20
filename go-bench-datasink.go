package main

import (
	"bufio"
	"flag"
	"fmt"
	redistimeseries "github.com/RedisTimeSeries/redistimeseries-go"
	"github.com/gomodule/redigo/redis"
	"golang.org/x/tools/benchmark/parse"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	tagsFlag = flag.String(
		"tag", "",
		"comma-separated list of key=value pairs to add to each document",
	)

	redistimeseriesEndpoint = flag.String(
		"redistimeseries-endpoint", "localhost:6379",
		"RedisTimeSeries URL into which the benchmark data should be inserted",
	)
	redistimeseriesAuth = flag.String(
		"redistimeseries-auth", "",
		"RedisTimeSeries auth",
	)

	gitRef = flag.String(
		"git-ref", "",
		"git ref (branch is a good one)",
	)

	keySuffix = flag.String(
		"key-suffix", "go-bench-datasink:",
		"RedisTimeSeries key suffix",
	)

	verboseFlag = flag.Bool("v", false, "Be verbose")
)

const (
	fieldName        = "name"
	fieldPkg         = "pkg"
	fieldGoVersion   = "go_version"
	fieldGOOS        = "goos"
	fieldGOARCH      = "goarch"
	fieldNSPerOp     = "ns_per_op"
	fieldGitRef      = "git_ref"
	fieldMeasurement = "measurement"
)

func main() {
	flag.Parse()
	if len(*gitRef) == 0 {
		fmt.Errorf("git-ref is required.")
		os.Exit(1)
	}
	pool := &redis.Pool{Dial: func() (redis.Conn, error) {
		return redis.Dial("tcp", *redistimeseriesEndpoint, redis.DialPassword(*redistimeseriesAuth))
	}}
	client := redistimeseries.NewClientFromPool(pool, "ts-client-1")
	tags := make(map[string]string)

	var pkg, goos, goarch string
	timestamp := time.Now().UTC()
	scanner := bufio.NewScanner(os.Stdin)
	datapointTs := timestamp.UTC().Unix() * 1000.0

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "pkg:"):
			pkg = strings.TrimSpace(line[len("pkg:"):])
		case strings.HasPrefix(line, "goos:"):
			goos = strings.TrimSpace(line[len("goos:"):])
		case strings.HasPrefix(line, "goarch:"):
			goarch = strings.TrimSpace(line[len("goarch:"):])
		default:
			if b, err := parse.ParseLine(line); err == nil {
				encodeIndexOp(
					client, b,
					pkg, goos, goarch,
					tags, datapointTs,
					*gitRef,
					*keySuffix,
				)
			}
		}
	}
	fmt.Println(pkg, goos, goarch)
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func encodeIndexOp(
	client *redistimeseries.Client,
	b *parse.Benchmark,
	pkg, goos, goarch string,
	tags map[string]string,
	timestamp int64,
	gitRef string,
	keySuffix string,
) {
	keyName := fmt.Sprintf("%s%s:%s:%s:%s:%s:%s:%s", keySuffix, goos, goarch, runtime.Version(), pkg, b.Name, gitRef, fieldNSPerOp)
	benchmarksSetName := fmt.Sprintf("%s%s:benchmarks", keySuffix, pkg)
	labels := map[string]string{
		fieldName:        b.Name,
		fieldPkg:         pkg,
		fieldGoVersion:   runtime.Version(),
		fieldGOOS:        goos,
		fieldGOARCH:      goarch,
		fieldGitRef:      gitRef,
		fieldMeasurement: fieldNSPerOp,
	}
	if b.Measured&parse.NsPerOp == 0 {
		return
	}
	metric := b.NsPerOp
	fmt.Println(timestamp, labels)
	client.CreateKeyWithOptions(keyName, redistimeseries.CreateOptions{Labels: labels})
	client.Add(keyName, timestamp, metric)
	client.Pool.Get().Do("SADD", benchmarksSetName, b.Name)
}
