package main

import (
	"errors"
	"flag"
	"github.com/Financial-Times/go-fthealth"
	"github.com/coreos/fleet/client"
	"github.com/coreos/fleet/schema"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/net/proxy"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	var (
		socksProxy    = flag.String("socks-proxy", "", "Use specified SOCKS proxy (e.g. localhost:2323)")
		fleetEndpoint = flag.String("fleetEndpoint", "", "Fleet API http endpoint: `http://host:port`")
		whitelist     = flag.String("timerBasedServices", "", "List of timer based services separated by a comma: deployer.service,image-cleaner.service,tunnel-register.service")
	)

	flag.Parse()

	fleetAPIClient, err := newFleetAPIClient(*fleetEndpoint, *socksProxy)
	if err != nil {
		panic(err)
	}
	wl := strings.Split(*whitelist, ",")
	log.Printf("whitelisted services: %v", wl)
	wlRegexp := make([]*regexp.Regexp, len(wl))
	for _, s := range wl {
		wlRegexp = append(wlRegexp, regexp.MustCompile(s))
	}
	handler := fleetUnitHealthHandler(fleetAPIClient, fleetUnitHealthChecker{wlRegexp})

	r := mux.NewRouter()
	r.HandleFunc("/", handler)
	r.HandleFunc("/__health", handler)

	err = http.ListenAndServe(":8080", handlers.LoggingHandler(os.Stdout, r))
	if err != nil {
		panic(err)
	}
}

func newFleetAPIClient(fleetEndpoint string, socksProxy string) (client.API, error) {
	u, err := url.Parse(fleetEndpoint)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{}

	if socksProxy != "" {
		log.Printf("using proxy %s\n", socksProxy)
		netDialler := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialer, err := proxy.SOCKS5("tcp", socksProxy, nil, netDialler)
		if err != nil {
			log.Fatalf("error with proxy %s: %v\n", socksProxy, err)
		}
		httpClient.Transport = &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		}
	}

	return client.NewHTTPClient(httpClient, *u)
}

func fleetUnitHealthHandler(fleetAPIClient client.API, checker fleetUnitHealthChecker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := []fthealth.Check{}
		unitStates, err := fleetAPIClient.UnitStates()
		if err != nil {
			panic(err)
		}
		for _, unitState := range unitStates {
			checks = append(checks, newFleetUnitHealthCheck(*unitState, checker))
		}
		fthealth.HandlerParallel("Coco Fleet Unit Healthcheck", "Checks the health of all fleet units", checks...)(w, r)
	}
}

func newFleetUnitHealthCheck(unitState schema.UnitState, checker fleetUnitHealthChecker) fthealth.Check {
	return fthealth.Check{
		Name:             unitState.Name + "_" + unitState.MachineID,
		Severity:         2,
		Checker:          func() error { return checker.Check(unitState) },
		TechnicalSummary: "This fleet unit is not in active state.",
		BusinessImpact:   "On its own this failure does not have a business impact but it represents a degradation of the cluster health.",
		PanicGuide:       "TO-DO",
	}
}

type fleetUnitHealthChecker struct {
	whitelist []*regexp.Regexp
}

func (f *fleetUnitHealthChecker) Check(unitState schema.UnitState) error {
	if "failed" == unitState.SystemdActiveState {
		return errors.New("Unit is in failed state.")
	}

	if "activating" == unitState.SystemdActiveState {
		return errors.New("Unit is in activating state.")
	}

	if "inactive" == unitState.SystemdActiveState && !isServiceWhitelisted(unitState.Name, f.whitelist) {
		return errors.New("Unit is in inactive state.")
	}

	return nil
}

func isServiceWhitelisted(serviceName string, whitelist []*regexp.Regexp) bool {
	for _, s := range whitelist {
		if s.MatchString(serviceName) {
			return true
		}
	}
	return false
}
