package generator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bilibili/kratos/tool/protobuf/pkg/generator"
	"github.com/bilibili/kratos/tool/protobuf/pkg/naming"
	"github.com/bilibili/kratos/tool/protobuf/pkg/tag"
	"github.com/bilibili/kratos/tool/protobuf/pkg/typemap"
	"github.com/bilibili/kratos/tool/protobuf/pkg/utils"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

type ginGenerator struct {
	generator.Base
	filesHandled int
}

// GinGenerator ginGenerator generator.
func GinGenerator() *ginGenerator {
	t := &ginGenerator{}
	return t
}

// Generate ...
func (t *ginGenerator) Generate(in *plugin.CodeGeneratorRequest) *plugin.CodeGeneratorResponse {
	t.Setup(in)

	// Showtime! Generate the response.
	resp := new(plugin.CodeGeneratorResponse)
	for _, f := range t.GenFiles {
		respFile := t.generateForFile(f)
		if respFile != nil {
			resp.File = append(resp.File, respFile)
		}
	}
	return resp
}

func (t *ginGenerator) generateForFile(file *descriptor.FileDescriptorProto) *plugin.CodeGeneratorResponse_File {
	resp := new(plugin.CodeGeneratorResponse_File)

	t.generateFileHeader(file, t.GenPkgName)
	t.generateImports(file)
	t.generatePathConstants(file)
	count := 0
	for i, service := range file.Service {
		count += t.generateGinInterface(file, service)
		t.generateGinRoute(file, service, i)
	}
	resp.Name = proto.String(naming.GoFileName(file, ".gin.go"))
	resp.Content = proto.String(t.FormattedOutput())
	t.Output.Reset()

	t.filesHandled++
	return resp
}

func (t *ginGenerator) generatePathConstants(file *descriptor.FileDescriptorProto) {
	t.P()
	for _, service := range file.Service {
		name := naming.ServiceName(service)
		for _, method := range service.Method {
			if !t.ShouldGenForMethod(file, service, method) {
				continue
			}
			apiInfo := t.GetHttpInfoCached(file, service, method)
			t.P(`var Path`, name, naming.MethodName(method), ` = "`, apiInfo.Path, `"`)
		}
		t.P()
	}
}

func (t *ginGenerator) generateFileHeader(file *descriptor.FileDescriptorProto, pkgName string) {
	t.P("// Code generated by protoc-gen-gin ", generator.Version, ", DO NOT EDIT.")
	t.P("// source: ", file.GetName())
	t.P()
	if t.filesHandled == 0 {
		// doc for the first file
		t.P("/*")
		t.P("Package ", t.GenPkgName, " is a generated blademaster stub package.")
		t.P("This code was generated with dangerous1990/protoc-gen-ginGenerator ", generator.Version, ".")
		t.P()
		comment, err := t.Reg.FileComments(file)
		if err == nil && comment.Leading != "" {
			for _, line := range strings.Split(comment.Leading, "\n") {
				line = strings.TrimPrefix(line, " ")
				// ensure we don't escape from the block comment
				line = strings.Replace(line, "*/", "* /", -1)
				t.P(line)
			}
			t.P()
		}
		t.P("It is generated from these files:")
		for _, f := range t.GenFiles {
			t.P("\t", f.GetName())
		}
		t.P("*/")
	}
	t.P(`package `, pkgName)
	t.P()
}

func (t *ginGenerator) generateImports(file *descriptor.FileDescriptorProto) {
	// if len(file.Service) == 0 {
	// 	return
	// }
	t.P(`import (`)
	// t.P(`	`,t.pkgs["context"], ` "context"`)
	t.P(`	"context"`)
	t.P()
	t.P(`	"github.com/gin-gonic/gin"`)
	t.P(`	"github.com/gin-gonic/gin/binding"`)

	t.P(`)`)
	// It's legal to import a message and use it as an input or output for a
	// method. Make sure to import the package of any such message. First, dedupe
	// them.
	deps := make(map[string]string) // Map of package name to quoted import path.
	deps = t.DeduceDeps(file)
	for pkg, importPath := range deps {
		for _, service := range file.Service {
			for _, method := range service.Method {
				inputType := t.GoTypeName(method.GetInputType())
				outputType := t.GoTypeName(method.GetOutputType())
				if strings.HasPrefix(pkg, outputType) || strings.HasPrefix(pkg, inputType) {
					t.P(`import `, pkg, ` `, importPath)
				}
			}
		}
	}
	if len(deps) > 0 {
		t.P()
	}
	t.P()
	t.P(`// to suppressed 'imported but not used warning'`)
	t.P(`var _ *gin.Context`)
	t.P(`var _ context.Context`)
	t.P(`var _ binding.StructValidator`)

}

// sectionComment Big header comments to makes it easier to visually parse a generated file.
func (t *ginGenerator) sectionComment(sectionTitle string) {
	t.P()
	t.P(`// `, strings.Repeat("=", len(sectionTitle)))
	t.P(`// `, sectionTitle)
	t.P(`// `, strings.Repeat("=", len(sectionTitle)))
	t.P()
}

func (t *ginGenerator) generateGinRoute(
	file *descriptor.FileDescriptorProto,
	service *descriptor.ServiceDescriptorProto,
	index int) {
	// old mode is generate xx.route.go in the http pkg
	// new mode is generate route code in the same .gin.go
	// route rule /x{department}/{project-name}/{path_prefix}/method_name
	// generate each route method
	servName := naming.ServiceName(service)
	versionPrefix := naming.GetVersionPrefix(t.GenPkgName)
	svcName := utils.LcFirst(utils.CamelCase(versionPrefix)) + servName + "Svc"
	t.P(`var `, svcName, ` `, servName, `GinServer`)

	type methodInfo struct {
		midwares      []string
		routeFuncName string
		apiInfo       *generator.HTTPInfo
		methodName    string
	}
	var methList []methodInfo
	var allMidwareMap = make(map[string]bool)
	for _, method := range service.Method {
		if !t.ShouldGenForMethod(file, service, method) {
			continue
		}
		var midwares []string
		comments, _ := t.Reg.MethodComments(file, service, method)
		tags := tag.GetTagsInComment(comments.Leading)
		if tag.GetTagValue("dynamic", tags) == "true" {
			continue
		}
		apiInfo := t.GetHttpInfoCached(file, service, method)

		midStr := tag.GetTagValue("midware", tags)
		if midStr != "" {
			midwares = strings.Split(midStr, ",")
			for _, m := range midwares {
				allMidwareMap[m] = true
			}
		}

		methName := naming.MethodName(method)
		inputType := t.GoTypeName(method.GetInputType())

		routeName := utils.LcFirst(utils.CamelCase(servName) +
			utils.CamelCase(methName))

		methList = append(methList, methodInfo{
			apiInfo:       apiInfo,
			midwares:      midwares,
			routeFuncName: routeName,
			methodName:    method.GetName(),
		})

		t.P(fmt.Sprintf("func %s (c *gin.Context) {", routeName))
		t.P(`	p := new(`, inputType, `)`)
		requestBinding := ""
		if t.hasHeaderTag(t.Reg.MessageDefinition(method.GetInputType())) {
			requestBinding = ", binding.Request"
		}
		t.P(`	if err := c.BindWith(p, binding.Default(c.Request.Method, c.Request.Header.Get("Content-Type"))` +
			requestBinding + `); err != nil {`)
		t.P(`		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})`)
		t.P(`		return`)
		t.P(`	}`)
		t.P(`	resp, err := `, svcName, `.`, methName, `(c, p)`)
		t.P(` 	if err !=nil {`)
		t.P(` 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})`)
		t.P(`		return`)
		t.P(`	}`)
		t.P(`	c.JSON(http.StatusOK, resp)`)
		t.P(`}`)
		t.P(``)
	}

	// generate route group
	var midList []string
	for m := range allMidwareMap {
		midList = append(midList, m+" gin.HandlerFunc")
	}

	sort.Strings(midList)

	var ginFuncName = fmt.Sprintf("Register%sGinServer", servName)
	t.P(`// `, ginFuncName, ` Register the gin route`)
	t.P(`func `, ginFuncName, `(e *gin.Engine, server `, servName, `GinServer) {`)
	t.P(svcName, ` = server`)
	for _, methInfo := range methList {
		t.P(`e.`, methInfo.apiInfo.HttpMethod, `("`, methInfo.apiInfo.NewPath, `",`, methInfo.routeFuncName, ` )`)
	}
	t.P(`	}`)
}

func (t *ginGenerator) hasHeaderTag(md *typemap.MessageDefinition) bool {
	if md.Descriptor.Field == nil {
		return false
	}
	for _, f := range md.Descriptor.Field {
		t := tag.GetMoreTags(f)
		if t != nil {
			st := reflect.StructTag(*t)
			if st.Get("request") != "" {
				return true
			}
			if st.Get("header") != "" {
				return true
			}
		}
	}
	return false
}

func (t *ginGenerator) generateGinInterface(file *descriptor.FileDescriptorProto, service *descriptor.ServiceDescriptorProto) int {
	count := 0
	servName := naming.ServiceName(service)
	t.P("// " + servName + "GinServer is the server API for " + servName + " service.")

	comments, err := t.Reg.ServiceComments(file, service)
	if err == nil {
		t.PrintComments(comments)
	}
	t.P(`type `, servName, `GinServer interface {`)
	for _, method := range service.Method {
		if !t.ShouldGenForMethod(file, service, method) {
			continue
		}
		count++
		t.generateInterfaceMethod(file, service, method, comments)
		t.P()
	}
	t.P(`}`)
	return count
}

func (t *ginGenerator) generateInterfaceMethod(file *descriptor.FileDescriptorProto,
	service *descriptor.ServiceDescriptorProto,
	method *descriptor.MethodDescriptorProto,
	comments typemap.DefinitionComments) {
	comments, err := t.Reg.MethodComments(file, service, method)

	methName := naming.MethodName(method)
	outputType := t.GoTypeName(method.GetOutputType())
	inputType := t.GoTypeName(method.GetInputType())
	tags := tag.GetTagsInComment(comments.Leading)
	if tag.GetTagValue("dynamic", tags) == "true" {
		return
	}

	if err == nil {
		t.PrintComments(comments)
	}

	respDynamic := tag.GetTagValue("dynamic_resp", tags) == "true"
	if respDynamic {
		t.P(fmt.Sprintf(`	%s(ctx context.Context, req *%s) (resp interface{}, err error)`,
			methName, inputType))
	} else {
		t.P(fmt.Sprintf(`	%s(ctx context.Context, req *%s) (resp *%s, err error)`,
			methName, inputType, outputType))
	}
}