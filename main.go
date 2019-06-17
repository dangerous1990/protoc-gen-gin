package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/bilibili/kratos/tool/protobuf/pkg/gen"
	"github.com/bilibili/kratos/tool/protobuf/pkg/generator"
	gingen "github.com/dangerous1990/protoc-gen-gin/generator"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(generator.Version)
		os.Exit(0)
	}
	g := gingen.GinGenerator()
	gen.Main(g)
}
