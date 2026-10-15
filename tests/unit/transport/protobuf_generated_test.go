//go:build unit

package transport_test

import (
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var _ = Describe("INTERNAL :: TRANSPORT :: GRPC :: PB :: GENERATED CONTRACT", func() {
	Describe("protobuf messages", func() {
		Context("success cases", func() {
			It("should provide safe defaults and preserve empty wire messages", func() {
				var messageCount, getterCount int
				protoregistry.GlobalTypes.RangeMessages(func(messageType protoreflect.MessageType) bool {
					if !strings.HasPrefix(string(messageType.Descriptor().FullName()), "ledger.v1.") {
						return true
					}
					messageCount++
					message := messageType.New().Interface()
					messageValue := reflect.ValueOf(message)
					for index := 0; index < messageValue.Type().NumMethod(); index++ {
						method := messageValue.Type().Method(index)
						switch {
						case strings.HasPrefix(method.Name, "Get"):
							getterCount++
							zeroResult := method.Func.Call([]reflect.Value{reflect.Zero(messageValue.Type())})
							valueResult := method.Func.Call([]reflect.Value{messageValue})
							Expect(zeroResult).To(HaveLen(1), messageType.Descriptor().FullName(), method.Name)
							Expect(zeroResult[0].Interface()).To(Equal(reflect.Zero(zeroResult[0].Type()).Interface()), messageType.Descriptor().FullName(), method.Name)
							Expect(valueResult[0].Interface()).To(Equal(reflect.Zero(valueResult[0].Type()).Interface()), messageType.Descriptor().FullName(), method.Name)
						case method.Name == "String":
							Expect(method.Func.Call([]reflect.Value{messageValue})).To(HaveLen(1))
						case method.Name == "ProtoMessage":
							method.Func.Call([]reflect.Value{reflect.Zero(messageValue.Type())})
						case method.Name == "Descriptor":
							Expect(method.Func.Call([]reflect.Value{messageValue})).To(HaveLen(2))
						}
					}
					wire, err := proto.Marshal(message)
					Expect(err).NotTo(HaveOccurred())
					Expect(wire).To(BeEmpty())
					decoded := messageType.New().Interface()
					Expect(proto.Unmarshal(wire, decoded)).To(Succeed())
					Expect(proto.Equal(message, decoded)).To(BeTrue())
					messageValue.MethodByName("Reset").Call(nil)
					Expect(proto.Equal(message, decoded)).To(BeTrue())
					return true
				})
				Expect(messageCount).To(BeNumerically(">", 0))
				Expect(getterCount).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("protobuf enums", func() {
		Context("success cases", func() {
			It("should preserve enum numbers, names, and descriptors", func() {
				var enumValueCount int
				protoregistry.GlobalTypes.RangeEnums(func(enumType protoreflect.EnumType) bool {
					if !strings.HasPrefix(string(enumType.Descriptor().FullName()), "ledger.v1.") {
						return true
					}
					values := enumType.Descriptor().Values()
					for index := 0; index < values.Len(); index++ {
						want := values.Get(index)
						value := reflect.ValueOf(enumType.New(want.Number()))
						enumValueCount++
						Expect(value.MethodByName("String").Call(nil)[0].String()).To(Equal(string(want.Name())))
						Expect(value.MethodByName("Number").Call(nil)[0].Int()).To(Equal(int64(want.Number())))
						Expect(value.MethodByName("Descriptor").Call(nil)[0].IsValid()).To(BeTrue())
						Expect(value.MethodByName("Type").Call(nil)[0].IsValid()).To(BeTrue())
						deprecatedDescriptor := value.MethodByName("EnumDescriptor").Call(nil)
						Expect(deprecatedDescriptor[0].IsNil()).To(BeFalse())
						Expect(value.MethodByName("Enum").Call(nil)[0].Elem().Int()).To(Equal(int64(want.Number())))
					}
					return true
				})
				Expect(enumValueCount).To(BeNumerically(">", 0))
			})
		})
	})
})
