package app

import (
	"flag"
	"fmt"
	"sync"
	"time"

	extend "github.com/dynamicgo/go-config-extend"
	"github.com/dynamicgo/go-config/source/envvar"
	"github.com/dynamicgo/go-config/source/file"
	flagsource "github.com/dynamicgo/go-config/source/flag"
	"github.com/dynamicgo/mesh"
	"github.com/dynamicgo/mesh/agent"
	"google.golang.org/grpc"

	config "github.com/dynamicgo/go-config"
	"github.com/dynamicgo/go-config/source"
	"github.com/dynamicgo/slf4go"
)

// ServiceDescriptor .
type ServiceDescriptor struct {
	Name           string
	Main           mesh.ServiceMain
	GRPCOptions    []grpc.ServerOption
	ServiceOptions []mesh.ServiceOption
}

type serviceRegister struct {
	slf4go.Logger
	sync.RWMutex
	services map[string]ServiceDescriptor
}

var globalServiceRegisterOnce sync.Once
var globalServiceRegister *serviceRegister

func initServiceRegister() {
	globalServiceRegister = &serviceRegister{
		Logger:   slf4go.Get("mesh-register"),
		services: make(map[string]ServiceDescriptor),
	}
}

// ImportService .
func ImportService(F ServiceDescriptor) {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	name := F.Name

	_, ok := globalServiceRegister.services[name]

	if ok {
		panic(fmt.Sprintf("duplicate import service %s", name))
	}

	globalServiceRegister.services[name] = F

	globalServiceRegister.InfoF("import service %s", name)
}

func getImportServices() map[string]ServiceDescriptor {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	services := make(map[string]ServiceDescriptor)

	for name, s := range globalServiceRegister.services {
		services[name] = s
	}
	return services
}

type app struct {
}

var logger = slf4go.Get("mesh-app")

// Run run mesh app
func Run() {

	defer func() {
		time.Sleep(time.Second * 2)

		println("mesh app exit")
	}()

	configfilepath := flag.String("config", "", "special the mesh app config file")

	flag.Parse()

	config := config.NewConfig()

	var sources []source.Source

	if *configfilepath == "" {
		logger.Info("mesh app config file -- not found")
	} else {
		sources = append(sources, file.NewSource(file.WithPath(*configfilepath)))
	}

	sources = append(sources, envvar.NewSource(envvar.WithPrefix("MESH")))

	sources = append(sources, flagsource.NewSource())

	if err := config.Load(sources...); err != nil {
		logger.Info(fmt.Sprintf("load config error: %s", err))
		return
	}

	logconfig, err := extend.SubConfig(config, "slf4go")

	if err != nil {
		logger.Info(fmt.Sprintf("get slf4go config error: %s", err))
		return
	}

	if err := slf4go.Load(logconfig); err != nil {
		logger.Info(fmt.Sprintf("load slf4go config error: %s", err))
		return
	}

	agent, err := agent.New(config)

	if err != nil {
		logger.Info(fmt.Sprintf("create agent error: %s", err))
		return
	}

	services := getImportServices()

	if len(services) == 0 {
		logger.Info(fmt.Sprintf("[%s] run nothing, exit", agent.Network().ID()))
		return
	}

	var wg sync.WaitGroup

	for name, f := range getImportServices() {
		go runService(&wg, agent, name, f)
	}

	wg.Wait()
}

func runService(wg *sync.WaitGroup, agent mesh.Agent, name string, f ServiceDescriptor) {
	defer wg.Done()
	wg.Add(1)

	logger.Info(fmt.Sprintf("service %s running...", name))

	mainf, op1, op2 := f.Main, f.GRPCOptions, f.ServiceOptions

	service, err := agent.RegisterService(name, op1...)

	if err != nil {
		logger.Info(fmt.Sprintf("service %s stop with err: %s", name, err))
		return
	}

	if err := service.Run(mainf, op2...); err != nil {
		logger.Info(fmt.Sprintf("service %s stop with err: %s", name, err))
	}
}
