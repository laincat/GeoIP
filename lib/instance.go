package lib

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"github.com/tailscale/hujson"
)

// Instance 是「一组输入 + 一组输出」的执行单元。
//
// 接口只声明真正被命令调用到的成员。上游把 ResetInput 与
// InitConfigFromBytes 也放进了接口，但全仓库没有任何外部调用点
// （ResetInput 的调用数是 0）—— 留着只会让实现被迫多写两个永不执行的方法。
// InitConfigFromBytes 保留为包内私有：它只是 InitConfig 的一个步骤。
type Instance interface {
	InitConfig(configFile string) error
	AddInput(InputConverter)
	AddOutput(OutputConverter)
	ResetOutput()
	RunInput(Container) error
	RunOutput(Container) error
	Run() error
}

type instance struct {
	input  []InputConverter
	output []OutputConverter
}

func NewInstance() (Instance, error) {
	return &instance{
		input:  make([]InputConverter, 0),
		output: make([]OutputConverter, 0),
	}, nil
}

func (i *instance) InitConfig(configFile string) error {
	var content []byte
	var err error

	configFile = strings.TrimSpace(configFile)
	if IsRemoteURI(configFile) {
		content, err = GetRemoteURLContent(configFile)
	} else {
		content, err = os.ReadFile(configFile)
	}
	if err != nil {
		return err
	}

	return i.initConfigFromBytes(content)
}

func (i *instance) initConfigFromBytes(content []byte) error {
	config := new(config)

	// Support JSON with comments and trailing commas
	content, _ = hujson.Standardize(content)

	if err := json.Unmarshal(content, &config); err != nil {
		return err
	}

	for _, input := range config.Input {
		i.input = append(i.input, input.converter)
	}

	for _, output := range config.Output {
		i.output = append(i.output, output.converter)
	}

	return nil
}

func (i *instance) AddInput(ic InputConverter) {
	i.input = append(i.input, ic)
}

func (i *instance) AddOutput(oc OutputConverter) {
	i.output = append(i.output, oc)
}

func (i *instance) ResetOutput() {
	i.output = make([]OutputConverter, 0)
}

func (i *instance) RunInput(container Container) error {
	var err error
	for _, ic := range i.input {
		container, err = ic.Input(container)
		if err != nil {
			return err
		}
	}

	return nil
}

func (i *instance) RunOutput(container Container) error {
	for _, oc := range i.output {
		if err := oc.Output(container); err != nil {
			return err
		}
	}

	return nil
}

func (i *instance) Run() error {
	if len(i.input) == 0 || len(i.output) == 0 {
		return errors.New("input type and output type must be specified")
	}

	container := NewContainer()

	if err := i.RunInput(container); err != nil {
		return err
	}

	if err := i.RunOutput(container); err != nil {
		return err
	}

	return nil
}
