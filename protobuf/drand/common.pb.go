// Code generated by protoc-gen-go. DO NOT EDIT.
// source: drand/common.proto

package drand

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type Empty struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Empty) Reset()         { *m = Empty{} }
func (m *Empty) String() string { return proto.CompactTextString(m) }
func (*Empty) ProtoMessage()    {}
func (*Empty) Descriptor() ([]byte, []int) {
	return fileDescriptor_e3db314147ee7469, []int{0}
}

func (m *Empty) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Empty.Unmarshal(m, b)
}
func (m *Empty) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Empty.Marshal(b, m, deterministic)
}
func (m *Empty) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Empty.Merge(m, src)
}
func (m *Empty) XXX_Size() int {
	return xxx_messageInfo_Empty.Size(m)
}
func (m *Empty) XXX_DiscardUnknown() {
	xxx_messageInfo_Empty.DiscardUnknown(m)
}

var xxx_messageInfo_Empty proto.InternalMessageInfo

// Identity holds the necessary information to contact a drand node
type Identity struct {
	Address              string   `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`
	Key                  []byte   `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	Tls                  bool     `protobuf:"varint,3,opt,name=tls,proto3" json:"tls,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Identity) Reset()         { *m = Identity{} }
func (m *Identity) String() string { return proto.CompactTextString(m) }
func (*Identity) ProtoMessage()    {}
func (*Identity) Descriptor() ([]byte, []int) {
	return fileDescriptor_e3db314147ee7469, []int{1}
}

func (m *Identity) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Identity.Unmarshal(m, b)
}
func (m *Identity) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Identity.Marshal(b, m, deterministic)
}
func (m *Identity) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Identity.Merge(m, src)
}
func (m *Identity) XXX_Size() int {
	return xxx_messageInfo_Identity.Size(m)
}
func (m *Identity) XXX_DiscardUnknown() {
	xxx_messageInfo_Identity.DiscardUnknown(m)
}

var xxx_messageInfo_Identity proto.InternalMessageInfo

func (m *Identity) GetAddress() string {
	if m != nil {
		return m.Address
	}
	return ""
}

func (m *Identity) GetKey() []byte {
	if m != nil {
		return m.Key
	}
	return nil
}

func (m *Identity) GetTls() bool {
	if m != nil {
		return m.Tls
	}
	return false
}

// GroupPacket represents a group
type GroupPacket struct {
	Nodes     []*Identity `protobuf:"bytes,1,rep,name=nodes,proto3" json:"nodes,omitempty"`
	Threshold uint32      `protobuf:"varint,2,opt,name=threshold,proto3" json:"threshold,omitempty"`
	// period in seconds
	Period               uint32   `protobuf:"varint,3,opt,name=period,proto3" json:"period,omitempty"`
	GenesisTime          uint64   `protobuf:"varint,4,opt,name=genesis_time,json=genesisTime,proto3" json:"genesis_time,omitempty"`
	TransitionTime       uint64   `protobuf:"varint,5,opt,name=transition_time,json=transitionTime,proto3" json:"transition_time,omitempty"`
	GenesisSeed          []byte   `protobuf:"bytes,6,opt,name=genesis_seed,json=genesisSeed,proto3" json:"genesis_seed,omitempty"`
	DistKey              [][]byte `protobuf:"bytes,7,rep,name=dist_key,json=distKey,proto3" json:"dist_key,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GroupPacket) Reset()         { *m = GroupPacket{} }
func (m *GroupPacket) String() string { return proto.CompactTextString(m) }
func (*GroupPacket) ProtoMessage()    {}
func (*GroupPacket) Descriptor() ([]byte, []int) {
	return fileDescriptor_e3db314147ee7469, []int{2}
}

func (m *GroupPacket) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GroupPacket.Unmarshal(m, b)
}
func (m *GroupPacket) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GroupPacket.Marshal(b, m, deterministic)
}
func (m *GroupPacket) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GroupPacket.Merge(m, src)
}
func (m *GroupPacket) XXX_Size() int {
	return xxx_messageInfo_GroupPacket.Size(m)
}
func (m *GroupPacket) XXX_DiscardUnknown() {
	xxx_messageInfo_GroupPacket.DiscardUnknown(m)
}

var xxx_messageInfo_GroupPacket proto.InternalMessageInfo

func (m *GroupPacket) GetNodes() []*Identity {
	if m != nil {
		return m.Nodes
	}
	return nil
}

func (m *GroupPacket) GetThreshold() uint32 {
	if m != nil {
		return m.Threshold
	}
	return 0
}

func (m *GroupPacket) GetPeriod() uint32 {
	if m != nil {
		return m.Period
	}
	return 0
}

func (m *GroupPacket) GetGenesisTime() uint64 {
	if m != nil {
		return m.GenesisTime
	}
	return 0
}

func (m *GroupPacket) GetTransitionTime() uint64 {
	if m != nil {
		return m.TransitionTime
	}
	return 0
}

func (m *GroupPacket) GetGenesisSeed() []byte {
	if m != nil {
		return m.GenesisSeed
	}
	return nil
}

func (m *GroupPacket) GetDistKey() [][]byte {
	if m != nil {
		return m.DistKey
	}
	return nil
}

type GroupRequest struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GroupRequest) Reset()         { *m = GroupRequest{} }
func (m *GroupRequest) String() string { return proto.CompactTextString(m) }
func (*GroupRequest) ProtoMessage()    {}
func (*GroupRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_e3db314147ee7469, []int{3}
}

func (m *GroupRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GroupRequest.Unmarshal(m, b)
}
func (m *GroupRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GroupRequest.Marshal(b, m, deterministic)
}
func (m *GroupRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GroupRequest.Merge(m, src)
}
func (m *GroupRequest) XXX_Size() int {
	return xxx_messageInfo_GroupRequest.Size(m)
}
func (m *GroupRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GroupRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GroupRequest proto.InternalMessageInfo

func init() {
	proto.RegisterType((*Empty)(nil), "drand.Empty")
	proto.RegisterType((*Identity)(nil), "drand.Identity")
	proto.RegisterType((*GroupPacket)(nil), "drand.GroupPacket")
	proto.RegisterType((*GroupRequest)(nil), "drand.GroupRequest")
}

func init() {
	proto.RegisterFile("drand/common.proto", fileDescriptor_e3db314147ee7469)
}

var fileDescriptor_e3db314147ee7469 = []byte{
	// 288 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x4c, 0x90, 0xc1, 0x4a, 0x33, 0x31,
	0x14, 0x85, 0xc9, 0x3f, 0x9d, 0x4e, 0x7b, 0x3b, 0x6d, 0x7f, 0xb2, 0x90, 0x08, 0x2e, 0xc6, 0x01,
	0x31, 0xab, 0x0a, 0xfa, 0x06, 0x82, 0xa8, 0xb8, 0x91, 0xe8, 0xca, 0x4d, 0x19, 0x9b, 0x8b, 0x0d,
	0xed, 0x24, 0x63, 0x72, 0xbb, 0x98, 0xf7, 0xf6, 0x01, 0x24, 0x69, 0x4b, 0xdd, 0xe5, 0x7c, 0x39,
	0x9c, 0xc3, 0x3d, 0xc0, 0xb5, 0x6f, 0xac, 0xbe, 0x59, 0xb9, 0xb6, 0x75, 0x76, 0xd1, 0x79, 0x47,
	0x8e, 0xe7, 0x89, 0xd5, 0x05, 0xe4, 0x0f, 0x6d, 0x47, 0x7d, 0xfd, 0x04, 0xa3, 0x67, 0x8d, 0x96,
	0x0c, 0xf5, 0x5c, 0x40, 0xd1, 0x68, 0xed, 0x31, 0x04, 0xc1, 0x2a, 0x26, 0xc7, 0xea, 0x28, 0xf9,
	0x7f, 0xc8, 0x36, 0xd8, 0x8b, 0x7f, 0x15, 0x93, 0xa5, 0x8a, 0xcf, 0x48, 0x68, 0x1b, 0x44, 0x56,
	0x31, 0x39, 0x52, 0xf1, 0x59, 0xff, 0x30, 0x98, 0x3c, 0x7a, 0xb7, 0xeb, 0x5e, 0x9b, 0xd5, 0x06,
	0x89, 0x5f, 0x41, 0x6e, 0x9d, 0xc6, 0x98, 0x95, 0xc9, 0xc9, 0xed, 0x7c, 0x91, 0x9a, 0x17, 0xc7,
	0x36, 0xb5, 0xff, 0xe5, 0x17, 0x30, 0xa6, 0xb5, 0xc7, 0xb0, 0x76, 0x5b, 0x9d, 0x0a, 0xa6, 0xea,
	0x04, 0xf8, 0x19, 0x0c, 0x3b, 0xf4, 0xc6, 0xe9, 0xd4, 0x34, 0x55, 0x07, 0xc5, 0x2f, 0xa1, 0xfc,
	0x42, 0x8b, 0xc1, 0x84, 0x25, 0x99, 0x16, 0xc5, 0xa0, 0x62, 0x72, 0xa0, 0x26, 0x07, 0xf6, 0x6e,
	0x5a, 0xe4, 0xd7, 0x30, 0x27, 0xdf, 0xd8, 0x60, 0xc8, 0x38, 0xbb, 0x77, 0xe5, 0xc9, 0x35, 0x3b,
	0xe1, 0x64, 0xfc, 0x93, 0x15, 0x10, 0xb5, 0x18, 0xa6, 0x2b, 0x8f, 0x59, 0x6f, 0x88, 0x9a, 0x9f,
	0xc3, 0x48, 0x9b, 0x40, 0xcb, 0x38, 0x42, 0x51, 0x65, 0xb2, 0x54, 0x45, 0xd4, 0x2f, 0xd8, 0xd7,
	0x33, 0x28, 0xd3, 0xd5, 0x0a, 0xbf, 0x77, 0x18, 0xe8, 0xbe, 0xf8, 0xd8, 0x4f, 0xfc, 0x39, 0x4c,
	0x83, 0xdf, 0xfd, 0x06, 0x00, 0x00, 0xff, 0xff, 0xbe, 0xc5, 0xfe, 0x0a, 0x86, 0x01, 0x00, 0x00,
}
