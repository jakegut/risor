package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/risor-io/risor/builtins"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
	"github.com/risor-io/risor/vm"

	modFmt "github.com/risor-io/risor/modules/fmt"
)

type State struct {
	Name    string
	Running bool
}

type Service struct {
	name       string
	running    bool
	startCount int
	stopCount  int
}

func (s *State) IsRunning() bool {
	return s.Running
}

func (s *Service) Start() error {
	if s.running {
		return fmt.Errorf("service %s already running", s.name)
	}
	s.running = true
	s.startCount++
	return nil
}

func (s *Service) Stop() error {
	if !s.running {
		return fmt.Errorf("service %s not running", s.name)
	}
	s.running = false
	s.stopCount++
	return nil
}

func (s *Service) SetName(name string) {
	s.name = name
}

func (s *Service) GetName() string {
	return s.name
}

func (s *Service) PrintState() {
	fmt.Printf("printing state... name: %s running %t\n", s.name, s.running)
}

func (s *Service) GetState() *State {
	return &State{
		Name:    s.name,
		Running: s.running,
	}
}

type contextKey string

const vmFuncKey = contextKey("yabs:vmfunc")

type VmFunc func() *vm.VirtualMachine

func newVMFunc(code *object.Code) VmFunc {
	return func() *vm.VirtualMachine {
		return vm.New(code)
	}
}

type taskRunner func()

var taskKV map[string]taskRunner = map[string]taskRunner{}

func runFunc(ctx context.Context, args ...object.Object) object.Object {
	strObj, ok := args[0].(*object.String)
	if !ok {
		return object.Errorf("expected a string")
	}
	name := strObj.String()

	fn, ok := args[1].(*object.Function)
	if !ok {
		return object.Errorf("expected a function")
	}

	taskKV[name] = func() {

		newVm, ok := ctx.Value(vmFuncKey).(VmFunc)
		if !ok {
			log.Fatalf("no vmfunc")
		}
		machine := newVm()

		svcProxy, err := object.NewProxy(&Service{})
		if err != nil {
			log.Fatal(err)
		}

		if err = machine.Run(ctx); err != nil {
			log.Fatal(err)
		}

		callFunc, ok := object.GetCallFunc(ctx)
		if !ok {
			object.Errorf("no call func")
		}

		if _, err := callFunc(ctx, fn, []object.Object{svcProxy}); err != nil {
			log.Fatalf("running func for %q: %s", name, err)
		}
	}

	return object.Nil
}

func compile(ctx context.Context, source string, builtins map[string]object.Object) (*object.Code, error) {

	ast, err := parser.Parse(ctx, source)
	if err != nil {
		return nil, err
	}

	compilerOpts := []compiler.Option{
		compiler.WithBuiltins(builtins),
	}
	comp, err := compiler.New(compilerOpts...)
	if err != nil {
		return nil, err
	}

	return comp.Compile(ast)
}

const defaultExample = `
for i := 0; i < 10; i++ {
	runFunc('svc-{i}', func(svc){
		i := i
		svc.SetName('svc-{i}')
		svc.Start()
		state := svc.GetState()
		print("STATE:", state, type(state))
		state.IsRunning()
	})
}
`

func main() {

	ctx := context.Background()

	// Build up options for Risor, including the proxy as a variable named "svc"
	runBuiltin := object.NewBuiltin("runFunc", runFunc)

	builtns := map[string]object.Object{"runFunc": runBuiltin}

	for k, v := range builtins.Builtins() {
		builtns[k] = v
	}

	// "for" loops don't work when I don't import this builtin
	for k, v := range modFmt.Builtins() {
		builtns[k] = v
	}

	code, err := compile(ctx, defaultExample, builtns)
	if err != nil {
		log.Fatal(err)
	}

	vmFunc := newVMFunc(code)

	ctx = context.WithValue(ctx, vmFuncKey, vmFunc)

	if err = vm.New(code).Run(ctx); err != nil {
		log.Fatal(err)
	}

	// Run the Risor code which can access the service as `svc`
	// if _, err := risor.Eval(ctx, defaultExample, risor.WithBuiltins(builtns)); err != nil {
	// 	log.Fatal(err)
	// }

	var wg sync.WaitGroup
	wg.Add(len(taskKV))
	for name, fn := range taskKV {
		log.Printf("running %q", name)
		go func(fn taskRunner) {
			defer wg.Done()
			fn()
		}(fn)
	}

	wg.Wait()
}
