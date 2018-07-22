package app

import (
	"flag"
	"fmt"
	"sync"

	extend "github.com/dynamicgo/go-config-extend"
	"github.com/dynamicgo/go-config/source/envvar"
	"github.com/dynamicgo/go-config/source/file"
	flagsource "github.com/dynamicgo/go-config/source/flag"
	"github.com/dynamicgo/mesh"
	"github.com/dynamicgo/mesh/agent"

	config "github.com/dynamicgo/go-config"
	"github.com/dynamicgo/go-config/source"
	"github.com/dynamicgo/slf4go"
)

// ServiceRegisterF .
type ServiceRegisterF func(agent mesh.Agent) error

type serviceRegister struct {
	slf4go.Logger
	sync.RWMutex
	services map[string]ServiceRegisterF
}

var globalServiceRegisterOnce sync.Once
var globalServiceRegister *serviceRegister

func initServiceRegister() {
	globalServiceRegister = &serviceRegister{
		Logger:   slf4go.Get("mesh-register"),
		services: make(map[string]ServiceRegisterF),
	}
}

// ImportService .
func ImportService(name string, F ServiceRegisterF) {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	_, ok := globalServiceRegister.services[name]

	if ok {
		panic(fmt.Sprintf("duplicate import service %s", name))
	}

	globalServiceRegister.services[name] = F

	globalServiceRegister.InfoF("import service %s", name)
}

func getImportServices() map[string]ServiceRegisterF {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	services := make(map[string]ServiceRegisterF)

	for name, s := range globalServiceRegister.services {
		services[name] = s
	}
	return services
}

type app struct {
}

// Run run mesh app
func Run() {

	configfilepath := flag.String("config", "", "special the mesh app config file")

	flag.Parse()

	config := config.NewConfig()

	var sources []source.Source

	if *configfilepath != "" {
		println("mesh app config file -- not found")
	} else {
		sources = append(sources, file.NewSource(file.WithPath(*configfilepath)))
	}

	sources = append(sources, envvar.NewSource(envvar.WithPrefix("MESH")))

	sources = append(sources, flagsource.NewSource())

	if err := config.Load(sources...); err != nil {
		println(fmt.Sprintf("load config error: %s", err))
		return
	}

	logconfig, err := extend.SubConfig(config, "slf4go")

	if err != nil {
		println(fmt.Sprintf("get slf4go config error: %s", err))
		return
	}

	if err := slf4go.Load(logconfig); err != nil {
		println(fmt.Sprintf("load slf4go config error: %s", err))
		return
	}

	agent, err := agent.New(config)

	if err != nil {
		println(fmt.Sprintf("create agent error: %s", err))
		return
	}

	var wg sync.WaitGroup

	for name, f := range getImportServices() {
		go runService(&wg, agent, name, f)
	}

	wg.Wait()
}

func runService(wg *sync.WaitGroup, agent mesh.Agent, name string, f ServiceRegisterF) {
	defer wg.Done()
	wg.Add(1)

	println(fmt.Sprintf("service %s running...", name))

	err := f(agent)

	println(fmt.Sprintf("service %s stop with err: %s", name, err))
}
