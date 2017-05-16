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

const (
	genericTecSum = "This app is not healthy, restart required"
	genericBusImp = "This app is not healthy, restart required"
	sidekickBisImp = "The associated app may not be receiving/forwarding requests properly"
	queueTecSum = "This unit is not healthy. Kafka, Zookeeper and the proxy are essential to publishing content to the website"
	queueBisImp = "Content is not being published; the website will become stale"
	aggHcTecSum = "Monitors health of services running in the cluster"
	aggHcBusSum = "Application health is not being monitored if this check return false"
	varnishTecSum = "Varnish cache not running, restart required"
	varnishBusSum = "All requests to access content will hit backend and may take longer than desired"
	vulcanTecSum = "Vulcan routes requests, restart required"
	vulcanBusSum = "Routing of requests on this machine may be failing"
	backupTecSum = "Database backup service not running; try restarting"
	backupBusImp = "Restoration of clusters will be slow without recent backups"
	loggerBusImp = "Logs from database will be lost"
	sidekickTecSum = "This sidekick unit is not healthy, restart required"
	mongoTecSum = "Stores all CAPI v2 content; restart required"
	mongoBusImp = "Customer requests may take longer than expected or return errors"
	transformerTecSum = "Stores all CAPI v2 content; restart required"
	transformerBusImp = "Customer requests may take longer than expected or return errors"
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
	for i, s := range wl {
		wlRegexp[i] = regexp.MustCompile(s)
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
	name := unitState.Name
	if strings.Contains(name, "sidekick") {
		return buildHealthcheck(unitState, checker, 3, sidekickTecSum, sidekickBisImp)
	} else if strings.Contains(name, "sidekick") {
		return buildHealthcheck(unitState, checker, 3, sidekickTecSum, sidekickBisImp)
	} else if strings.Contains(name, "kafka") || strings.Contains(name, "zookeeper") {
		return buildHealthcheck(unitState, checker, 1, queueTecSum, queueBisImp)
	} else if strings.Contains(name, "varnish") {
		return buildHealthcheck(unitState, checker, 1, varnishTecSum, varnishBusSum)
	} else if strings.Contains(name, "vulcan") {
		return buildHealthcheck(unitState, checker, 1, vulcanTecSum, vulcanBusSum)
	} else if strings.Contains(name, "aggregate-healthcheck") {
		return buildHealthcheck(unitState, checker, 1, aggHcTecSum, aggHcBusSum)
	} else if strings.Contains(name, "timer") {
		return buildHealthcheck(unitState, checker, 2, genericTecSum, "Database backups will not run if this service is unhealthy")
	} else if strings.Contains(name, "backup") {
		return buildHealthcheck(unitState, checker, 2, backupTecSum, backupBusImp)
	} else if strings.Contains(name, "mongodb") {
		return buildHealthcheck(unitState, checker, 1, mongoTecSum, mongoBusImp)
	} else if strings.Contains(name, "transformer") {
		return buildHealthcheck(unitState, checker, 2, transformerTecSum, transformerBusImp)
	} else if strings.Contains(name, "logger") {
		return buildHealthcheck(unitState, checker, 2, genericTecSum, loggerBusImp)
	} else if strings.Contains(name, "burrow") {
		return buildHealthcheck(unitState, checker, 2, genericTecSum, "Kafka lagcheck service will not report kafka lags")
	} else if strings.Contains(name, "elb") || strings.Contains(name, "tunnel-registrator") {
		return buildHealthcheck(unitState, checker, 2, "Should only alert on cluster creation, try restarting", "Should only alert on cluster creation")
	} else if strings.Contains(name, "splunk-forwarder") || strings.Contains(name, "diamond") || strings.Contains(name, "image-cleaner") {
		return buildHealthcheck(unitState, checker, 2, genericTecSum, "")
	} else {
		return genericHealthcheck(unitState, checker, "View this services healthcheck, from main cluster health page, for recovery information and panic guide")
	}
	return fthealth.Check{}
}

func buildHealthcheck(unitState schema.UnitState, checker fleetUnitHealthChecker, severity uint8, technicalSummary string, businessImpact string) fthealth.Check {
	return fthealth.Check{
		Name:             unitState.Name + "_" + unitState.MachineID,
		Severity:         severity,
		Checker:          func() error { return checker.Check(unitState) },
		TechnicalSummary: technicalSummary,
		BusinessImpact:   businessImpact,
		PanicGuide:       "https://dewey.ft.com/fleet-unit-healthcheck.html",
	}
}

func genericHealthcheck(unitState schema.UnitState, checker fleetUnitHealthChecker, message string) fthealth.Check {
	return fthealth.Check{
		Name:             unitState.Name + "_" + unitState.MachineID,
		Checker:          func() error { return checker.Check(unitState) },
		TechnicalSummary: message,
		BusinessImpact:   message,
		PanicGuide:       message,
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
