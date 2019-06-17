# proto-gen-gin

## require
* protoc
* go 1.12+

brew install protoc 
## go install
install protoc-gen-gin to your $GOPATH
## run example
 protoc -Iexample/third_party -Iexample -Iexample/third_party/github.com/gogo/protobuf --gin_out=example  api.proto
