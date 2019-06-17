# proto-gen-gin

## require
* protoc
 
## go install
install protoc-gen-gin to your $GOPATH
## protoc 
brew install protoc 
## run example
 protoc -Iexample/third_party -Iexample -Iexample/third_party/github.com/gogo/protobuf --gin_out=example  api.proto
