package core

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type addrinfoData struct {
	family, pfamily, socktype, protocol int64
	ip                                  string
	port                                int64
	unixPath                            string
	canonname                           *object.EmeraldValue
}

type socketOptionData struct {
	family, level, optname int64
	data                   string
	kind                   string
}

type socketIfaddrData struct {
	index int64
	name  string
	flags int64
	addr  *object.EmeraldValue
	broad *object.EmeraldValue
	mask  *object.EmeraldValue
}

type socketData struct {
	family, socktype, protocol  int64
	localIP, remoteIP           string
	localPath, remotePath       string
	localPort, remotePort       int64
	fd                          int64
	closed                      bool
	bound, listening, connected bool
	readClosed, writeClosed     bool
	shutdownRead, shutdownWrite bool
	peerClosed                  bool
	doNotReverseLookup          bool
	ipv6Only                    bool
	nonblock                    bool
	buffer, oobBuffer           string
	sentIO                      *object.EmeraldValue
	readWaiters, acceptWaiters  []*object.EmeraldValue
	options                     map[string]*socketOptionData
}

var socketServers map[int64]*object.EmeraldValue
var socketFDs map[int64]*object.EmeraldValue
var socketExternalData map[*object.EmeraldValue]*socketData
var unixSocketServers map[string]*object.EmeraldValue
var socketNextPort int64
var socketNextFD int64
var socketDoNotReverseLookup = true

func socketQueueWaiter(waiters *[]*object.EmeraldValue, thread *object.EmeraldValue) {
	for _, waiter := range *waiters {
		if waiter == thread {
			return
		}
	}
	*waiters = append(*waiters, thread)
}

func socketWakeWaiters(waiters *[]*object.EmeraldValue, all bool) {
	for len(*waiters) > 0 {
		waiter := (*waiters)[0]
		*waiters = (*waiters)[1:]
		data := threadValueData(waiter)
		if data == nil || data.finished {
			continue
		}
		data.stopped = false
		queuePendingThread(waiter)
		if !all {
			return
		}
	}
}

func socketWaitForRead(receiver *object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(receiver)
	current := threadClassCurrent(nil)
	for d.buffer == "" && d.oobBuffer == "" && !d.peerClosed && !d.shutdownRead && !d.closed && !d.readClosed {
		currentData := threadValueData(current)
		if currentData == nil {
			return nil
		}
		if currentData.block == nil {
			if len(pendingThreads) == 0 {
				return nil
			}
			runNextPendingThread()
			continue
		}
		currentData.stopped = true
		socketQueueWaiter(&d.readWaiters, current)
		if SuspendCurrentThread != nil {
			result := SuspendCurrentThread()
			currentData.stopped = false
			if result != nil && result.Type == object.ValueException {
				return result
			}
			continue
		}
		runNextPendingThread()
		currentData.stopped = false
		return nil
	}
	return nil
}

func socketWaitForAccept(receiver *object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(receiver)
	current := threadClassCurrent(nil)
	for receiverInstanceVarMap(receiver)["@__pending_socket"] == nil && !d.closed {
		currentData := threadValueData(current)
		if currentData == nil {
			return nil
		}
		if currentData.block == nil {
			if len(pendingThreads) == 0 {
				return nil
			}
			runNextPendingThread()
			continue
		}
		currentData.stopped = true
		currentData.blockedLabel = receiver.Class.Name + "#accept"
		socketQueueWaiter(&d.acceptWaiters, current)
		if SuspendCurrentThread != nil {
			result := SuspendCurrentThread()
			currentData.stopped = false
			currentData.blockedLabel = ""
			if result != nil && result.Type == object.ValueException {
				return result
			}
			continue
		}
		runNextPendingThread()
		currentData.stopped = false
		currentData.blockedLabel = ""
		return nil
	}
	return nil
}

func installSocketAddrinfo(objectClass *object.Class, socket *object.Class) {
	if socket == nil || socket.Constants["SOCK_STREAM"] != nil {
		return
	}
	if R.Classes["IO::TimeoutError"] == nil {
		klass := object.NewClass("IO::TimeoutError")
		klass.SuperClass = R.Classes["IOError"]
		R.Classes["IO::TimeoutError"] = klass
		if ioClass := R.Classes["IO"]; ioClass != nil {
			ioClass.DefineConstant("TimeoutError", classEmeraldValue(klass))
		}
	}
	for name, value := range map[string]int64{
		"AF_UNSPEC": 0, "PF_UNSPEC": 0, "AF_UNIX": 1, "PF_UNIX": 1,
		"AF_INET": 2, "PF_INET": 2, "AF_INET6": 10, "PF_INET6": 10,
		"SOCK_STREAM": 1, "SOCK_DGRAM": 2, "SOCK_RAW": 3, "SOCK_RDM": 4, "SOCK_SEQPACKET": 5, "SOCK_PACKET": 10,
		"IPPROTO_IP": 0, "IPPROTO_HOPOPTS": 0, "IPPROTO_TCP": 6, "IPPROTO_UDP": 17, "AI_CANONNAME": 2,
		"AI_PASSIVE": 1, "NI_NUMERICHOST": 1, "NI_NUMERICSERV": 2,
	} {
		socket.DefineConstant(name, newInt(value))
	}
	socket.DefineConstant("INADDR_ANY", newInt(0))
	socket.DefineConstant("SHUT_RD", newInt(0))
	socket.DefineConstant("SHUT_WR", newInt(1))
	socket.DefineConstant("SHUT_RDWR", newInt(2))
	socket.DefineConstant("MSG_PEEK", newInt(2))
	socket.DefineConstant("MSG_OOB", newInt(1))
	socket.DefineConstant("SOL_SOCKET", newInt(1))
	socket.DefineConstant("SO_KEEPALIVE", newInt(9))
	socket.DefineConstant("SO_REUSEADDR", newInt(2))
	socket.DefineConstant("SO_TYPE", newInt(3))
	socket.DefineConstant("SO_BROADCAST", newInt(6))
	socket.DefineConstant("SO_SNDBUF", newInt(7))
	socket.DefineConstant("SO_OOBINLINE", newInt(10))
	socket.DefineConstant("SO_LINGER", newInt(13))
	socket.DefineConstant("IP_TTL", newInt(2))
	socket.DefineConstant("IPPROTO_IPV6", newInt(41))
	socket.DefineConstant("IPV6_V6ONLY", newInt(26))
	socket.DefineConstant("TCP_NODELAY", newInt(1))
	constants := object.NewModule("Socket::Constants")
	for name, value := range socket.Constants {
		constants.DefineConstant(name, value)
	}
	socket.DefineConstant("Constants", &object.EmeraldValue{Type: object.ValueModule, Data: constants, Class: R.Classes["Module"]})
	socket.DefineClassMethod("sockaddr_in", &object.Method{Name: "sockaddr_in", Fn: socketSockaddrIn, Arity: 2})
	socket.DefineClassMethod("pack_sockaddr_in", &object.Method{Name: "pack_sockaddr_in", Fn: socketSockaddrIn, Arity: 2})
	socket.DefineClassMethod("sockaddr_un", &object.Method{Name: "sockaddr_un", Fn: socketSockaddrUn, Arity: 1})
	socket.DefineClassMethod("pack_sockaddr_un", &object.Method{Name: "pack_sockaddr_un", Fn: socketSockaddrUn, Arity: 1})
	socket.DefineClassMethod("getservbyport", &object.Method{Name: "getservbyport", Fn: socketGetservbyport, Arity: -1})
	socket.DefineClassMethod("getservbyname", &object.Method{Name: "getservbyname", Fn: socketGetservbyname, Arity: -1})
	socket.DefineClassMethod("gethostname", &object.Method{Name: "gethostname", Fn: socketGethostname, Arity: 0})
	socket.DefineClassMethod("gethostbyname", &object.Method{Name: "gethostbyname", Fn: socketGethostbyname, Arity: 1})
	socket.DefineClassMethod("gethostbyaddr", &object.Method{Name: "gethostbyaddr", Fn: socketGethostbyaddr, Arity: -1})
	socket.DefineClassMethod("getaddrinfo", &object.Method{Name: "getaddrinfo", Fn: socketGetaddrinfo, Arity: -1})
	socket.DefineClassMethod("getnameinfo", &object.Method{Name: "getnameinfo", Fn: socketGetnameinfo, Arity: -1})
	socket.DefineClassMethod("unpack_sockaddr_in", &object.Method{Name: "unpack_sockaddr_in", Fn: socketUnpackSockaddrIn, Arity: 1})
	socket.DefineClassMethod("unpack_sockaddr_un", &object.Method{Name: "unpack_sockaddr_un", Fn: socketUnpackSockaddrUn, Arity: 1})
	socket.DefineClassMethod("udp_server_sockets", &object.Method{Name: "udp_server_sockets", Fn: socketUDPServerSockets, Arity: -1})
	socket.DefineClassMethod("udp_server_recv", &object.Method{Name: "udp_server_recv", Fn: socketUDPServerRecv, Arity: 1})
	socket.DefineClassMethod("udp_server_loop_on", &object.Method{Name: "udp_server_loop_on", Fn: socketUDPServerLoopOn, Arity: 1})
	socket.DefineClassMethod("udp_server_loop", &object.Method{Name: "udp_server_loop", Fn: socketUDPServerLoop, Arity: -1})
	socket.DefineClassMethod("tcp_server_sockets", &object.Method{Name: "tcp_server_sockets", Fn: socketTCPServerSockets, Arity: -1})
	socket.DefineClassMethod("tcp_server_loop", &object.Method{Name: "tcp_server_loop", Fn: socketTCPServerLoop, Arity: -1})
	udpSource := object.NewClass("Socket::UDPSource")
	udpSource.SuperClass = objectClass
	R.Classes["Socket::UDPSource"] = udpSource
	socket.DefineConstant("UDPSource", classEmeraldValue(udpSource))

	klass := object.NewClass("Addrinfo")
	klass.SuperClass = R.Classes["Object"]
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: addrinfoNew, Arity: -1})
	klass.DefineClassMethod("ip", &object.Method{Name: "ip", Fn: addrinfoIP, Arity: 1})
	klass.DefineClassMethod("tcp", &object.Method{Name: "tcp", Fn: addrinfoTCP, Arity: 2})
	klass.DefineClassMethod("udp", &object.Method{Name: "udp", Fn: addrinfoUDP, Arity: 2})
	klass.DefineClassMethod("unix", &object.Method{Name: "unix", Fn: addrinfoUnix, Arity: -1})
	klass.DefineClassMethod("getaddrinfo", &object.Method{Name: "getaddrinfo", Fn: addrinfoGetaddrinfo, Arity: -1})
	for name, def := range map[string]struct {
		fn    interface{}
		arity int
	}{
		"afamily": {addrinfoAFamily, 0}, "pfamily": {addrinfoPFamily, 0}, "socktype": {addrinfoSocktype, 0}, "protocol": {addrinfoProtocol, 0},
		"ip_address": {addrinfoIPAddress, 0}, "ip_port": {addrinfoIPPort, 0}, "unix_path": {addrinfoUnixPath, 0},
		"ip?": {addrinfoIPQuestion, 0}, "ipv4?": {addrinfoIPv4Question, 0}, "ipv6?": {addrinfoIPv6Question, 0}, "unix?": {addrinfoUnixQuestion, 0},
		"ipv4_loopback?": {addrinfoIPv4Loopback, 0}, "ipv4_multicast?": {addrinfoIPv4Multicast, 0}, "ipv4_private?": {addrinfoIPv4Private, 0},
		"ipv6_loopback?": {addrinfoIPv6Loopback, 0}, "ipv6_multicast?": {addrinfoIPv6Multicast, 0},
		"getnameinfo": {addrinfoGetnameinfo, -1}, "ipv6_to_ipv4": {addrinfoIPv6ToIPv4, 0},
		"bind": {addrinfoBind, 0}, "connect_from": {addrinfoConnectFrom, -1}, "connect_to": {addrinfoConnectTo, -1},
		"to_sockaddr": {addrinfoToSockaddr, 0}, "to_s": {addrinfoToSockaddr, 0}, "inspect_sockaddr": {addrinfoInspectSockaddr, 0}, "inspect": {addrinfoInspect, 0},
		"ip_unpack": {addrinfoIPUnpack, 0}, "family_addrinfo": {addrinfoFamilyAddrinfo, -1},
		"canonname": {addrinfoCanonname, 0}, "marshal_dump": {addrinfoMarshalDump, 0}, "marshal_load": {addrinfoMarshalLoad, 1},
	} {
		klass.DefineMethod(name, &object.Method{Name: name, Fn: def.fn, Arity: def.arity})
	}
	R.Classes["Addrinfo"] = klass
	value := classEmeraldValue(klass)
	objectClass.DefineConstant("Addrinfo", value)
	AssignConstantName(classEmeraldValue(objectClass), "Addrinfo", value)
	if R.Classes["SocketError"] == nil {
		errClass := object.NewClass("SocketError")
		errClass.SuperClass = R.Classes["StandardError"]
		R.Classes["SocketError"] = errClass
		objectClass.DefineConstant("SocketError", classEmeraldValue(errClass))
	}
	installSocketOption(socket)
	installSocketIfaddr(socket)
	installSocketRuntime(objectClass, socket)
}

func installSocketIfaddr(socket *object.Class) {
	klass := object.NewClass("Socket::Ifaddr")
	klass.SuperClass = R.Classes["Object"]
	klass.DefineMethod("ifindex", &object.Method{Name: "ifindex", Fn: socketIfaddrIndex, Arity: 0})
	klass.DefineMethod("name", &object.Method{Name: "name", Fn: socketIfaddrName, Arity: 0})
	klass.DefineMethod("flags", &object.Method{Name: "flags", Fn: socketIfaddrFlags, Arity: 0})
	klass.DefineMethod("addr", &object.Method{Name: "addr", Fn: socketIfaddrAddr, Arity: 0})
	klass.DefineMethod("broadaddr", &object.Method{Name: "broadaddr", Fn: socketIfaddrBroadaddr, Arity: 0})
	klass.DefineMethod("netmask", &object.Method{Name: "netmask", Fn: socketIfaddrNetmask, Arity: 0})
	R.Classes["Socket::Ifaddr"] = klass
	socket.DefineConstant("Ifaddr", classEmeraldValue(klass))
	socket.DefineClassMethod("getifaddrs", &object.Method{Name: "getifaddrs", Fn: socketGetifaddrs, Arity: 0})
}

func socketIfaddrDataOf(receiver *object.EmeraldValue) *socketIfaddrData {
	data, _ := receiver.Data.(*socketIfaddrData)
	return data
}

func socketIfaddrIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(socketIfaddrDataOf(receiver).index)
}
func socketIfaddrName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(socketIfaddrDataOf(receiver).name)
}
func socketIfaddrFlags(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(socketIfaddrDataOf(receiver).flags)
}
func socketIfaddrAddr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketIfaddrDataOf(receiver).addr
}
func socketIfaddrBroadaddr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketIfaddrDataOf(receiver).broad
}
func socketIfaddrNetmask(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketIfaddrDataOf(receiver).mask
}
func socketGetifaddrs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	address := addrinfoValue(&addrinfoData{family: 2, pfamily: 2, ip: "127.0.0.1"})
	broadcast := addrinfoValue(&addrinfoData{family: 2, pfamily: 2, ip: "127.255.255.255"})
	mask := addrinfoValue(&addrinfoData{family: 2, pfamily: 2, ip: "255.0.0.0"})
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &socketIfaddrData{index: 1, name: "lo", flags: 1, addr: address, broad: broadcast, mask: mask}, Class: R.Classes["Socket::Ifaddr"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{value}, Class: R.Classes["Array"]}
}

func installSocketRuntime(objectClass *object.Class, socket *object.Class) {
	socketServers = make(map[int64]*object.EmeraldValue)
	socketFDs = make(map[int64]*object.EmeraldValue)
	socketExternalData = make(map[*object.EmeraldValue]*socketData)
	unixSocketServers = make(map[string]*object.EmeraldValue)
	socketNextPort = 41000
	socketNextFD = 100
	basic := object.NewClass("BasicSocket")
	basic.SuperClass = R.Classes["IO"]
	basic.DefineClassMethod("do_not_reverse_lookup", &object.Method{Name: "do_not_reverse_lookup", Fn: socketDoNotReverseLookupGet, Arity: 0})
	basic.DefineClassMethod("do_not_reverse_lookup=", &object.Method{Name: "do_not_reverse_lookup=", Fn: socketDoNotReverseLookupSet, Arity: 1})
	basic.DefineClassMethod("for_fd", &object.Method{Name: "for_fd", Fn: socketForFD, Arity: 1})
	basic.DefineMethod("do_not_reverse_lookup", &object.Method{Name: "do_not_reverse_lookup", Fn: socketInstanceDoNotReverseLookupGet, Arity: 0})
	basic.DefineMethod("do_not_reverse_lookup=", &object.Method{Name: "do_not_reverse_lookup=", Fn: socketInstanceDoNotReverseLookupSet, Arity: 1})
	basic.DefineMethod("peeraddr", &object.Method{Name: "peeraddr", Fn: socketPeeraddr, Arity: -1})
	basic.DefineMethod("local_address", &object.Method{Name: "local_address", Fn: socketLocalAddress, Arity: 0})
	basic.DefineMethod("remote_address", &object.Method{Name: "remote_address", Fn: socketRemoteAddress, Arity: 0})
	basic.DefineMethod("connect_address", &object.Method{Name: "connect_address", Fn: socketConnectAddress, Arity: 0})
	basic.DefineMethod("fileno", &object.Method{Name: "fileno", Fn: socketFileno, Arity: 0})
	basic.DefineMethod("to_io", &object.Method{Name: "to_io", Fn: socketSelf, Arity: 0})
	basic.DefineMethod("autoclose=", &object.Method{Name: "autoclose=", Fn: socketReturnArgument, Arity: 1})
	basic.DefineMethod("getsockopt", &object.Method{Name: "getsockopt", Fn: socketGetsockopt, Arity: 2})
	basic.DefineMethod("setsockopt", &object.Method{Name: "setsockopt", Fn: socketSetsockopt, Arity: -1})
	basic.DefineMethod("close_read", &object.Method{Name: "close_read", Fn: socketCloseRead, Arity: 0})
	basic.DefineMethod("close_write", &object.Method{Name: "close_write", Fn: socketCloseWrite, Arity: 0})
	basic.DefineMethod("shutdown", &object.Method{Name: "shutdown", Fn: socketShutdown, Arity: 1})
	basic.DefineMethod("getsockname", &object.Method{Name: "getsockname", Fn: socketGetsockname, Arity: 0})
	basic.DefineMethod("getpeername", &object.Method{Name: "getpeername", Fn: socketGetpeername, Arity: 0})
	basic.DefineMethod("send", &object.Method{Name: "send", Fn: socketSend, Arity: -1})
	basic.DefineMethod("listen", &object.Method{Name: "listen", Fn: socketListen, Arity: 1})
	basic.DefineMethod("read_nonblock", &object.Method{Name: "read_nonblock", Fn: socketRead, Arity: -1})
	basic.DefineMethod("recv_nonblock", &object.Method{Name: "recv_nonblock", Fn: socketRecvNonblock, Arity: -1})
	basic.DefineMethod("write_nonblock", &object.Method{Name: "write_nonblock", Fn: socketWrite, Arity: 1})
	basic.DefineMethod("puts", &object.Method{Name: "puts", Fn: socketPuts, Arity: -1})
	basic.DefineMethod("nonblock?", &object.Method{Name: "nonblock?", Fn: socketNonblockGet, Arity: 0})
	basic.DefineMethod("nonblock=", &object.Method{Name: "nonblock=", Fn: socketNonblockSet, Arity: 1})
	basic.DefineMethod("sendmsg", &object.Method{Name: "sendmsg", Fn: socketSendmsg, Arity: -1})
	basic.DefineMethod("sendmsg_nonblock", &object.Method{Name: "sendmsg_nonblock", Fn: socketSendmsgNonblock, Arity: -1})
	basic.DefineMethod("recvmsg", &object.Method{Name: "recvmsg", Fn: socketRecvmsg, Arity: -1})
	basic.DefineMethod("recvmsg_nonblock", &object.Method{Name: "recvmsg_nonblock", Fn: socketRecvmsgNonblock, Arity: -1})
	for name, def := range map[string]struct {
		fn    interface{}
		arity int
	}{"close": {socketClose, 0}, "closed?": {socketClosed, 0}, "binmode?": {socketTrue, 0}, "nonblock?": {socketTrue, 0}, "close_on_exec?": {socketTrue, 0}, "lineno": {socketLineno, 0}, "write": {socketWrite, 1}, "<<": {socketAppend, 1}, "recv": {socketRecv, -1}, "read": {socketRead, -1}, "gets": {socketGets, -1}, "print": {socketPrint, -1}, "ioctl": {socketIoctl, 2}} {
		basic.DefineMethod(name, &object.Method{Name: name, Fn: def.fn, Arity: def.arity})
	}
	R.Classes["BasicSocket"] = basic
	objectClass.DefineConstant("BasicSocket", classEmeraldValue(basic))
	socket.SuperClass = basic
	if unixSocket := R.Classes["UNIXSocket"]; unixSocket != nil {
		unixSocket.SuperClass = basic
		unixSocket.DefineMethod("path", &object.Method{Name: "path", Fn: unixSocketPath, Arity: 0})
		unixSocket.DefineMethod("addr", &object.Method{Name: "addr", Fn: unixSocketAddr, Arity: 0})
		unixSocket.DefineMethod("peeraddr", &object.Method{Name: "peeraddr", Fn: unixSocketPeeraddr, Arity: 0})
		unixSocket.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: unixSocketInspect, Arity: 0})
		unixSocket.DefineMethod("send_io", &object.Method{Name: "send_io", Fn: unixSocketSendIO, Arity: 1})
		unixSocket.DefineMethod("recv_io", &object.Method{Name: "recv_io", Fn: unixSocketRecvIO, Arity: -1})
		unixSocket.DefineMethod("getpeereid", &object.Method{Name: "getpeereid", Fn: unixSocketGetpeereid, Arity: 0})
		unixSocket.DefineMethod("recvfrom", &object.Method{Name: "recvfrom", Fn: unixSocketRecvfrom, Arity: -1})
		unixSocket.DefineClassMethod("pair", &object.Method{Name: "pair", Fn: unixSocketClassPair, Arity: 0})
		unixSocket.DefineClassMethod("socketpair", &object.Method{Name: "socketpair", Fn: unixSocketClassPair, Arity: 0})
		if unixServer := R.Classes["UNIXServer"]; unixServer != nil {
			unixServer.SuperClass = unixSocket
			unixServer.DefineMethod("accept", &object.Method{Name: "accept", Fn: unixServerAccept, Arity: 0})
			unixServer.DefineMethod("accept_nonblock", &object.Method{Name: "accept_nonblock", Fn: unixServerAcceptNonblock, Arity: -1})
			unixServer.DefineMethod("sysaccept", &object.Method{Name: "sysaccept", Fn: unixServerSysaccept, Arity: 0})
		}
	}
	socket.DefineClassMethod("new", &object.Method{Name: "new", Fn: socketClassNew, Arity: -1})
	socket.DefineClassMethod("tcp", &object.Method{Name: "tcp", Fn: socketClassTCP, Arity: -1})
	socket.DefineClassMethod("pair", &object.Method{Name: "pair", Fn: socketClassPair, Arity: -1})
	socket.DefineClassMethod("socketpair", &object.Method{Name: "socketpair", Fn: socketClassPair, Arity: -1})
	for name, def := range map[string]struct {
		fn    interface{}
		arity int
	}{"close": {socketClose, 0}, "closed?": {socketClosed, 0}, "binmode?": {socketTrue, 0}, "nonblock?": {socketNonblockGet, 0}, "nonblock=": {socketNonblockSet, 1}, "close_on_exec?": {socketTrue, 0}, "lineno": {socketLineno, 0}, "ipv6only!": {socketIPv6Only, 0}, "local_address": {socketLocalAddress, 0}, "connect_address": {socketConnectAddress, 0}, "bind": {socketBind, 1}, "connect": {socketConnect, 1}, "connect_nonblock": {socketConnectNonblock, -1}, "listen": {socketListen, 1}, "accept": {socketAccept, 0}, "accept_nonblock": {socketAcceptNonblock, -1}, "sysaccept": {socketSysaccept, 0}, "write": {socketWrite, 1}, "recv": {socketRecv, -1}, "recvfrom": {socketRecvfrom, -1}, "recvfrom_nonblock": {socketRecvfromNonblock, -1}, "gets": {socketGets, -1}, "print": {socketPrint, -1}, "ioctl": {socketIoctl, 2}} {
		socket.DefineMethod(name, &object.Method{Name: name, Fn: def.fn, Arity: def.arity})
	}
	ipSocket := object.NewClass("IPSocket")
	ipSocket.SuperClass = basic
	ipSocket.DefineClassMethod("getaddress", &object.Method{Name: "getaddress", Fn: ipSocketGetaddress, Arity: 1})
	ipSocket.DefineMethod("addr", &object.Method{Name: "addr", Fn: udpSocketAddr, Arity: -1})
	ipSocket.DefineMethod("recvfrom", &object.Method{Name: "recvfrom", Fn: ipSocketRecvfrom, Arity: -1})
	ipSocket.DefineMethod("recvfrom_nonblock", &object.Method{Name: "recvfrom_nonblock", Fn: ipSocketRecvfromNonblock, Arity: -1})
	R.Classes["IPSocket"] = ipSocket
	objectClass.DefineConstant("IPSocket", classEmeraldValue(ipSocket))
	udp := object.NewClass("UDPSocket")
	udp.SuperClass = ipSocket
	udp.DefineClassMethod("new", &object.Method{Name: "new", Fn: udpSocketNew, Arity: -1})
	udp.DefineClassMethod("open", &object.Method{Name: "open", Fn: udpSocketNew, Arity: -1})
	udp.DefineMethod("addr", &object.Method{Name: "addr", Fn: udpSocketAddr, Arity: 0})
	udp.DefineMethod("bind", &object.Method{Name: "bind", Fn: udpSocketBind, Arity: 2})
	udp.DefineMethod("connect", &object.Method{Name: "connect", Fn: udpSocketConnect, Arity: 2})
	udp.DefineMethod("send", &object.Method{Name: "send", Fn: udpSocketSend, Arity: -1})
	udp.DefineMethod("fileno", &object.Method{Name: "fileno", Fn: udpSocketFileno, Arity: 0})
	udp.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: udpSocketInspect, Arity: 0})
	R.Classes["UDPSocket"] = udp
	objectClass.DefineConstant("UDPSocket", classEmeraldValue(udp))
	tcpServer := object.NewClass("TCPServer")
	tcpServer.DefineClassMethod("new", &object.Method{Name: "new", Fn: tcpServerNew, Arity: -1})
	tcpServer.DefineMethod("addr", &object.Method{Name: "addr", Fn: tcpServerAddr, Arity: 0})
	tcpServer.DefineMethod("accept", &object.Method{Name: "accept", Fn: tcpServerAccept, Arity: 0})
	tcpServer.DefineMethod("accept_nonblock", &object.Method{Name: "accept_nonblock", Fn: tcpServerAcceptNonblock, Arity: -1})
	tcpServer.DefineMethod("sysaccept", &object.Method{Name: "sysaccept", Fn: tcpServerSysaccept, Arity: 0})
	R.Classes["TCPServer"] = tcpServer
	objectClass.DefineConstant("TCPServer", classEmeraldValue(tcpServer))
	tcpSocket := object.NewClass("TCPSocket")
	tcpSocket.SuperClass = ipSocket
	tcpSocket.DefineClassMethod("new", &object.Method{Name: "new", Fn: tcpSocketOpen, Arity: -1})
	tcpSocket.DefineClassMethod("open", &object.Method{Name: "open", Fn: tcpSocketOpen, Arity: -1})
	tcpSocket.DefineClassMethod("gethostbyname", &object.Method{Name: "gethostbyname", Fn: tcpSocketGethostbyname, Arity: 1})
	tcpServer.SuperClass = tcpSocket
	R.Classes["TCPSocket"] = tcpSocket
	objectClass.DefineConstant("TCPSocket", classEmeraldValue(tcpSocket))
}
func socketDomainArg(value *object.EmeraldValue, kind string) (int64, *object.EmeraldValue) {
	if n, ok := socketNamedValue(value, kind); ok {
		return n, nil
	}
	raw, ok, converted, e := evalCoerceToString(value)
	if e != nil {
		return 0, e
	}
	if ok {
		if n, valid := socketNamedValue(rubyString(raw), kind); valid {
			return n, nil
		}
	}
	if converted && !ok {
		return 0, typeError("can't convert into String")
	}
	return 0, newRuntimeException(R.Classes["SocketError"], "unsupported socket domain")
}
func socketClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	family, e := socketDomainArg(args[0], "family")
	if e != nil {
		return e
	}
	kind, e := socketDomainArg(args[1], "socktype")
	if e != nil {
		return e
	}
	protocol := int64(0)
	if len(args) > 2 {
		n, ok := valueToInteger(args[2])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		protocol = n
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: kind, protocol: protocol, localIP: "0.0.0.0", doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
}
func socketClassPair(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	family, errVal := socketDomainArg(args[0], "family")
	if errVal != nil {
		return errVal
	}
	socktype, errVal := socketDomainArg(args[1], "socktype")
	if errVal != nil {
		return errVal
	}
	klass, _ := receiver.Data.(*object.Class)
	a := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: socktype, doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
	b := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: socktype, doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
	receiverInstanceVarMap(a)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: b, Class: R.Classes["Object"]}
	receiverInstanceVarMap(b)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: a, Class: R.Classes["Object"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{a, b}, Class: R.Classes["Array"]}
}
func udpSocketNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	family := int64(2)
	var e *object.EmeraldValue
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	if len(args) == 1 {
		family, e = socketDomainArg(args[0], "family")
		if e != nil {
			return e
		}
		if family != 2 && family != 10 {
			return newRuntimeException(R.Classes["SystemCallError"], "Address family not supported")
		}
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: 2, localIP: "0.0.0.0", doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
}
func ipSocketGetaddress(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	host, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	if parsed, err := netip.ParseAddr(host); err == nil {
		return rubyString(parsed.String())
	}
	local, _ := os.Hostname()
	if host == local || host == "localhost" {
		return rubyString("127.0.0.1")
	}
	return newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Name or service not known")
}
func socketDataOf(r *object.EmeraldValue) *socketData {
	if d, ok := r.Data.(*socketData); ok && d != nil {
		return d
	}
	if d := socketExternalData[r]; d != nil {
		return d
	}
	d := &socketData{family: 1, socktype: 1, doNotReverseLookup: socketDoNotReverseLookup}
	if vars := receiverInstanceVarMap(r); vars != nil {
		if path := vars["@path"]; path != nil && path.Type == object.ValueString {
			d.remotePath = stringRawValue(path)
			if r.Class == R.Classes["UNIXServer"] {
				d.localPath = d.remotePath
			}
		}
	}
	socketExternalData[r] = d
	return d
}
func socketTrue(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return R.TrueVal
}
func socketNonblockGet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(socketDataOf(r).nonblock)
}
func socketNonblockSet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	socketDataOf(r).nonblock = a[0].IsTruthy()
	return a[0]
}
func socketDoNotReverseLookupGet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(socketDoNotReverseLookup)
}
func socketDoNotReverseLookupSet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	socketDoNotReverseLookup = a[0].IsTruthy()
	return a[0]
}
func socketInstanceDoNotReverseLookupGet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(socketDataOf(r).doNotReverseLookup)
}
func socketInstanceDoNotReverseLookupSet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	socketDataOf(r).doNotReverseLookup = a[0].IsTruthy()
	return a[0]
}
func socketPeeraddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.remoteIP == "" {
		klass := R.Classes["Errno::ENOTCONN"]
		if klass == nil {
			klass = R.Classes["SystemCallError"]
		}
		return newRuntimeException(klass, "Socket is not connected")
	}
	host := d.remoteIP
	if host == "" {
		host = "127.0.0.1"
	}
	name := host
	reverse, errVal := socketReverseLookupFlag(d, a)
	if errVal != nil {
		return errVal
	}
	if reverse {
		name, _ = os.Hostname()
	}
	family := "AF_INET"
	if d.family == 10 {
		family = "AF_INET6"
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(family), newInt(d.remotePort), rubyString(name), rubyString(host)}, Class: R.Classes["Array"]}
}
func socketReverseLookupFlag(d *socketData, args []*object.EmeraldValue) (bool, *object.EmeraldValue) {
	if len(args) == 0 {
		return !d.doNotReverseLookup, nil
	}
	if args[0].Type == object.ValueBool {
		return args[0].IsTruthy(), nil
	}
	name, ok, errVal := MethodNameFromValueWithError(args[0])
	if errVal != nil {
		return false, errVal
	}
	if ok && name == "hostname" {
		return true, nil
	}
	return false, NewArgumentError("invalid reverse_lookup flag")
}
func unixSocketPath(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(socketDataOf(r).localPath)
}
func unixSocketAddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString("AF_UNIX"), rubyString(socketDataOf(r).localPath)}, Class: R.Classes["Array"]}
}
func unixSocketPeeraddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if r.Class == R.Classes["UNIXServer"] || d.remotePath == "" && !d.connected {
		klass := R.Classes["Errno::ENOTCONN"]
		if klass == nil {
			klass = R.Classes["SystemCallError"]
		}
		return newRuntimeException(klass, "Socket is not connected")
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString("AF_UNIX"), rubyString(d.remotePath)}, Class: R.Classes["Array"]}
}

func unixSocketGetpeereid(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{newInt(int64(os.Geteuid())), newInt(int64(os.Getegid()))}, Class: R.Classes["Array"]}
}
func unixSocketInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return rubyString("#<UNIXSocket:(closed)>")
	}
	fd, _ := valueToInteger(socketFileno(r))
	return rubyString(fmt.Sprintf("#<UNIXSocket:fd %d>", fd))
}
func unixSocketSendIO(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil {
		if peer, ok := stored.Data.(*object.EmeraldValue); ok {
			socketDataOf(peer).sentIO = args[0]
			return R.NilVal
		}
	}
	return newRuntimeException(R.Classes["IOError"], "not connected")
}
func unixSocketRecvIO(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	source := socketDataOf(r).sentIO
	if source == nil {
		return newRuntimeException(R.Classes["IOError"], "no IO available")
	}
	klass := R.Classes["IO"]
	if len(args) > 0 && args[0].Type == object.ValueClass {
		klass, _ = args[0].Data.(*object.Class)
	}
	return &object.EmeraldValue{Type: source.Type, Data: source.Data, Class: klass}
}
func unixSocketRecvfrom(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := socketRecv(r, args...)
	if data.Type == object.ValueException || data.Type == object.ValueNil {
		return data
	}
	d := socketDataOf(r)
	path := ""
	if d.remotePath != "" {
		path = d.remotePath
	}
	if path == "" {
		if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil {
			if peer, ok := stored.Data.(*object.EmeraldValue); ok {
				path = socketDataOf(peer).localPath
			}
		}
	}
	address := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString("AF_UNIX"), rubyString(path)}, Class: R.Classes["Array"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{data, address}, Class: R.Classes["Array"]}
}
func unixSocketClassPair(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	a := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 1, socktype: 1, connected: true, doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
	b := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 1, socktype: 1, connected: true, doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
	receiverInstanceVarMap(a)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: b, Class: R.Classes["Object"]}
	receiverInstanceVarMap(b)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: a, Class: R.Classes["Object"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{a, b}, Class: R.Classes["Array"]}
}
func registerUnixServerSocket(value *object.EmeraldValue, path string) {
	if socketExternalData == nil {
		socketExternalData = make(map[*object.EmeraldValue]*socketData)
	}
	if unixSocketServers == nil {
		unixSocketServers = make(map[string]*object.EmeraldValue)
	}
	d := &socketData{family: 1, socktype: 1, localPath: path, bound: true, listening: true, doNotReverseLookup: socketDoNotReverseLookup}
	socketExternalData[value] = d
	unixSocketServers[path] = value
}
func registerUnixClientSocket(value *object.EmeraldValue, path string) {
	if socketExternalData == nil {
		socketExternalData = make(map[*object.EmeraldValue]*socketData)
	}
	d := &socketData{family: 1, socktype: 1, remotePath: path, doNotReverseLookup: socketDoNotReverseLookup}
	socketExternalData[value] = d
	if server := unixSocketServers[path]; server != nil {
		peer := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 1, socktype: 1, localPath: path, nonblock: true, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["UNIXSocket"]}
		receiverInstanceVarMap(server)["@__pending_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(value)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(peer)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: value, Class: R.Classes["Object"]}
		socketWakeWaiters(&socketDataOf(server).acceptWaiters, false)
	}
}
func unixServerAccept(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if result := socketWaitForAccept(r); result != nil {
		return result
	}
	if socketDataOf(r).closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if stored := receiverInstanceVarMap(r)["@__pending_socket"]; stored != nil {
		peer, _ := stored.Data.(*object.EmeraldValue)
		delete(receiverInstanceVarMap(r), "@__pending_socket")
		return peer
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 1, socktype: 1, nonblock: true, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["UNIXSocket"]}
}
func unixServerAcceptNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiverInstanceVarMap(r)["@__pending_socket"] != nil {
		return unixServerAccept(r)
	}
	if len(args) > 0 && args[0].Type == object.ValueHash {
		return rubySymbol("wait_readable")
	}
	return newRuntimeException(R.Classes["Errno::EAGAIN"], "Resource temporarily unavailable")
}
func unixServerSysaccept(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return socketFileno(unixServerAccept(r))
}
func socketReturnArgument(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return a[0]
}
func socketIPv6Only(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	socketDataOf(r).ipv6Only = true
	return R.NilVal
}
func socketFileno(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.fd == 0 {
		socketNextFD++
		d.fd = socketNextFD
		socketFDs[d.fd] = r
	}
	return newInt(d.fd)
}
func socketForFD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	fd, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	source := socketFDs[fd]
	if source == nil {
		return NewArgumentError("invalid file descriptor")
	}
	klass, _ := receiver.Data.(*object.Class)
	value := &object.EmeraldValue{Type: object.ValueObject, Data: socketDataOf(source), Class: klass}
	if peer := receiverInstanceVarMap(source)["@__peer_socket"]; peer != nil {
		receiverInstanceVarMap(value)["@__peer_socket"] = peer
	}
	return value
}
func socketSelf(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue { return r }
func socketListen(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if _, ok := valueToInteger(args[0]); !ok {
		return typeError("no implicit conversion into Integer")
	}
	if socketDataOf(r).socktype == 2 {
		klass := R.Classes["Errno::EOPNOTSUPP"]
		if klass == nil {
			klass = R.Classes["SystemCallError"]
		}
		return newRuntimeException(klass, "Operation not supported")
	}
	d := socketDataOf(r)
	if !d.bound {
		d.bound = true
		if d.localPort == 0 {
			socketNextPort++
			d.localPort = socketNextPort
		}
		if d.localIP == "" {
			if d.family == 10 {
				d.localIP = "::"
			} else {
				d.localIP = "0.0.0.0"
			}
		}
		socketServers[d.localPort] = r
	}
	d.listening = true
	return newInt(0)
}
func socketLineno(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(0)
}
func socketClose(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	d.closed = true
	socketWakeWaiters(&d.readWaiters, true)
	socketWakeWaiters(&d.acceptWaiters, true)
	if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil {
		if peer, ok := stored.Data.(*object.EmeraldValue); ok {
			peerData := socketDataOf(peer)
			peerData.peerClosed = true
			socketWakeWaiters(&peerData.readWaiters, true)
		}
	}
	return R.NilVal
}
func socketCloseRead(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	d.readClosed = true
	if d.writeClosed {
		d.closed = true
	}
	return R.NilVal
}
func socketCloseWrite(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	d.writeClosed = true
	if d.readClosed {
		d.closed = true
	}
	return R.NilVal
}
func socketShutdown(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	mode := int64(2)
	ok := true
	if len(args) > 0 {
		mode, ok = valueToInteger(args[0])
	}
	if !ok {
		name, valid, errVal := MethodNameFromValueWithError(args[0])
		if errVal != nil {
			return errVal
		}
		if !valid {
			return typeError("no implicit conversion into String")
		}
		name = strings.TrimPrefix(name, "SHUT_")
		switch name {
		case "RD":
			mode = 0
		case "WR":
			mode = 1
		case "RDWR":
			mode = 2
		default:
			return newRuntimeException(R.Classes["SocketError"], "invalid shutdown mode")
		}
	}
	if mode < 0 || mode > 2 {
		return NewArgumentError("invalid shutdown mode")
	}
	d := socketDataOf(r)
	d.shutdownRead = mode == 0 || mode == 2
	d.shutdownWrite = mode == 1 || mode == 2
	if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil && d.shutdownWrite {
		if peer, valid := stored.Data.(*object.EmeraldValue); valid {
			peerData := socketDataOf(peer)
			peerData.shutdownRead = true
			socketWakeWaiters(&peerData.readWaiters, true)
		}
	}
	return newInt(0)
}
func socketClosed(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(socketDataOf(r).closed)
}
func socketLocalAddress(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.family == 1 {
		return addrinfoValue(&addrinfoData{family: 1, pfamily: 1, socktype: 1, unixPath: d.localPath})
	}
	return addrinfoValue(&addrinfoData{family: d.family, pfamily: d.family, socktype: d.socktype, protocol: 0, ip: d.localIP, port: d.localPort})
}
func socketConnectAddress(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.family == 1 {
		if d.localPath == "" {
			return newRuntimeException(R.Classes["SocketError"], "unbound socket")
		}
		return socketLocalAddress(r)
	}
	if !d.bound && d.localPort == 0 {
		return newRuntimeException(R.Classes["SocketError"], "unbound socket")
	}
	address := socketLocalAddress(r)
	data := addrinfoDataOf(address)
	if data.ip == "0.0.0.0" {
		data.ip = "127.0.0.1"
	} else if data.ip == "::" {
		data.ip = "::1"
	}
	return address
}
func socketGetsockname(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return addrinfoToSockaddr(socketLocalAddress(r))
}
func socketGetpeername(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.family != 1 && d.remoteIP == "" {
		klass := R.Classes["Errno::ENOTCONN"]
		if klass == nil {
			klass = R.Classes["SystemCallError"]
		}
		return newRuntimeException(klass, "Socket is not connected")
	}
	return addrinfoToSockaddr(socketRemoteAddress(r))
}
func socketRemoteAddress(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.family == 1 {
		return addrinfoValue(&addrinfoData{family: 1, pfamily: 1, socktype: 1, unixPath: d.remotePath})
	}
	return addrinfoValue(&addrinfoData{family: d.family, pfamily: d.family, socktype: d.socktype, protocol: 0, ip: d.remoteIP, port: d.remotePort})
}
func socketBind(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var address *addrinfoData
	if args[0].Class == R.Classes["Addrinfo"] {
		address = addrinfoDataOf(args[0])
	} else {
		if args[0].Type != object.ValueString {
			return typeError("no implicit conversion into String")
		}
		var e *object.EmeraldValue
		address, e = unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if e != nil {
			return e
		}
	}
	d := socketDataOf(r)
	if d.bound {
		return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	if address.family != 1 && address.ip != "127.0.0.1" && address.ip != "0.0.0.0" && address.ip != "::1" && address.ip != "::" {
		return newRuntimeException(R.Classes["Errno::EADDRNOTAVAIL"], "Cannot assign requested address")
	}
	if address.port == 1 {
		return newRuntimeException(R.Classes["Errno::EACCES"], "Permission denied")
	}
	d.family = address.family
	if address.family == 1 {
		d.localPath = address.unixPath
		d.bound = true
		unixSocketServers[d.localPath] = r
		return newInt(0)
	}
	d.localIP = address.ip
	d.localPort = address.port
	d.bound = true
	if d.localPort == 0 {
		socketNextPort++
		d.localPort = socketNextPort
	}
	socketServers[d.localPort] = r
	return newInt(0)
}
func socketConnect(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if socketDataOf(r).connected {
		return newRuntimeException(R.Classes["Errno::EISCONN"], "Transport endpoint is already connected")
	}
	var address *addrinfoData
	if args[0].Class == R.Classes["Addrinfo"] {
		address = addrinfoDataOf(args[0])
	} else {
		if args[0].Type != object.ValueString {
			return typeError("no implicit conversion into String")
		}
		var errVal *object.EmeraldValue
		address, errVal = unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if errVal != nil {
			return errVal
		}
	}
	d := socketDataOf(r)
	d.family, d.remoteIP, d.remotePort = address.family, address.ip, address.port
	d.connected = true
	if d.localIP == "" || d.localIP == "0.0.0.0" {
		d.localIP = address.ip
	}
	if d.localPort == 0 {
		socketNextPort++
		d.localPort = socketNextPort
	}
	if server := socketServers[d.remotePort]; server != nil && d.socktype == 1 {
		sd := socketDataOf(server)
		peer := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: d.family, socktype: 1, localIP: sd.localIP, localPort: sd.localPort, remoteIP: d.localIP, remotePort: d.localPort, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["Socket"]}
		receiverInstanceVarMap(server)["@__pending_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(r)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(peer)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: r, Class: R.Classes["Object"]}
		socketWakeWaiters(&sd.acceptWaiters, false)
	}
	socketServers[d.localPort] = r
	return newInt(0)
}
func socketConnectNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0].Type != object.ValueString && args[0].Class != R.Classes["Addrinfo"] {
		return typeError("no implicit conversion into String")
	}
	d := socketDataOf(r)
	exceptionless := len(args) == 2 && args[1].Type == object.ValueHash
	if d.connected {
		if exceptionless {
			return newInt(0)
		}
		return newRuntimeException(R.Classes["Errno::EISCONN"], "Transport endpoint is already connected")
	}
	result := socketConnect(r, args[0])
	if result.Type == object.ValueException || d.socktype == 2 {
		return result
	}
	if exceptionless {
		return rubySymbol("wait_writable")
	}
	return newRuntimeException(R.Classes["Errno::EINPROGRESS"], "Operation now in progress")
}
func socketClassTCP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 5 {
		return NewArgumentError("wrong number of arguments")
	}
	host, e := httpString(args[0])
	if e != nil {
		return e
	}
	port, e := addrinfoPort(args[1])
	if e != nil {
		return e
	}
	value := socketClassNew(receiver, rubyString("INET"), rubyString("STREAM"))
	d := socketDataOf(value)
	d.remoteIP = host
	d.remotePort = port
	d.localIP = "127.0.0.1"
	socketNextPort++
	d.localPort = socketNextPort
	if len(args) >= 4 {
		local, e := httpString(args[2])
		if e != nil {
			return e
		}
		d.localIP = local
	}
	if server := socketServers[port]; server != nil {
		peer := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 2, socktype: 1, localIP: host, localPort: port, remoteIP: d.localIP, remotePort: d.localPort, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["Socket"]}
		receiverInstanceVarMap(server)["@__pending_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(value)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(peer)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: value, Class: R.Classes["Object"]}
		socketWakeWaiters(&socketDataOf(server).acceptWaiters, false)
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), value)
		d.closed = true
		return result
	}
	return value
}
func socketAccept(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if !d.bound || !d.listening {
		return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	stored := receiverInstanceVarMap(r)["@__pending_socket"]
	if stored == nil {
		if result := socketWaitForAccept(r); result != nil {
			return result
		}
		if d.closed {
			return newRuntimeException(R.Classes["IOError"], "closed stream")
		}
		stored = receiverInstanceVarMap(r)["@__pending_socket"]
	}
	if stored == nil {
		peer := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: 2, socktype: 1, localIP: "127.0.0.1", buffer: "CLOSE", doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["Socket"]}
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{peer, socketLocalAddress(peer)}, Class: R.Classes["Array"]}
	}
	peer, _ := stored.Data.(*object.EmeraldValue)
	delete(receiverInstanceVarMap(r), "@__pending_socket")
	address := socketRemoteAddress(peer)
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{peer, address}, Class: R.Classes["Array"]}
}
func socketAcceptNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if !d.bound || !d.listening {
		return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	if receiverInstanceVarMap(r)["@__pending_socket"] != nil {
		return socketAccept(r)
	}
	if len(args) > 0 && args[0].Type == object.ValueHash {
		return rubySymbol("wait_readable")
	}
	return newRuntimeException(R.Classes["Errno::EAGAIN"], "Resource temporarily unavailable")
}
func socketSysaccept(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if !d.bound || !d.listening {
		return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	accepted := socketAccept(r)
	values, _ := accepted.Data.([]*object.EmeraldValue)
	if len(values) != 2 {
		return newRuntimeException(R.Classes["IOError"], "accept failed")
	}
	fd := socketFileno(values[0])
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{fd, values[1]}, Class: R.Classes["Array"]}
}
func socketWrite(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if d := socketDataOf(r); d.closed || d.writeClosed || d.shutdownWrite {
		if d.shutdownWrite {
			klass := R.Classes["Errno::EPIPE"]
			if klass == nil {
				klass = R.Classes["IOError"]
			}
			return newRuntimeException(klass, "Broken pipe")
		}
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	raw, e := httpString(args[0])
	if e != nil {
		return e
	}
	if socketDataOf(r).socktype == 2 && len(raw) > 65507 {
		return newRuntimeException(R.Classes["Errno::EMSGSIZE"], "Message too long")
	}
	if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil {
		if peer, ok := stored.Data.(*object.EmeraldValue); ok {
			peerData := socketDataOf(peer)
			peerData.buffer += raw
			socketWakeWaiters(&peerData.readWaiters, false)
		}
	} else if d := socketDataOf(r); d.remotePort != 0 && socketServers[d.remotePort] != nil {
		serverData := socketDataOf(socketServers[d.remotePort])
		serverData.buffer += raw
		serverData.remoteIP, serverData.remotePort = d.localIP, d.localPort
		socketWakeWaiters(&serverData.readWaiters, false)
	} else {
		d := socketDataOf(r)
		d.buffer += raw
		socketWakeWaiters(&d.readWaiters, false)
	}
	return newInt(int64(len(raw)))
}
func socketAppend(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := socketWrite(r, args...)
	if result.Type == object.ValueException {
		return result
	}
	return r
}
func ipSocketRecvfrom(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := socketRecv(r, args...)
	if data.Type == object.ValueException || data.Type == object.ValueNil {
		return data
	}
	d := socketDataOf(r)
	if d.socktype == 1 && data.Type == object.ValueString && stringRawValue(data) == "" && d.shutdownRead {
		return R.NilVal
	}
	address := socketPeeraddr(r)
	if address.Type == object.ValueException {
		address = R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{data, address}, Class: R.Classes["Array"]}
}
func ipSocketRecvfromNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.buffer == "" {
		if d.socktype == 1 && !d.connected && d.remotePort == 0 {
			return newRuntimeException(R.Classes["Errno::ENOTCONN"], "Transport endpoint is not connected")
		}
		if len(args) > 0 && args[len(args)-1].Type == object.ValueHash {
			return rubySymbol("wait_readable")
		}
		return newRuntimeException(R.Classes["IO::EAGAINWaitReadable"], "Resource temporarily unavailable")
	}
	return ipSocketRecvfrom(r, args...)
}
func socketRecvfrom(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := socketRecv(r, args...)
	if data.Type == object.ValueException || data.Type == object.ValueSymbol || data.Type == object.ValueNil {
		return data
	}
	d := socketDataOf(r)
	address := addrinfoValue(&addrinfoData{family: d.family, pfamily: d.family, socktype: d.socktype, protocol: 0, ip: d.remoteIP, port: d.remotePort})
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{data, address}, Class: R.Classes["Array"]}
}
func socketRecvfromNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if result := socketNonblockReadGuard(r, args...); result != nil {
		return result
	}
	return socketRecvfrom(r, args...)
}
func socketRecvmsg(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && args[len(args)-1].Type == object.ValueHash && socketDataOf(r).buffer == "" {
		return rubySymbol("wait_readable")
	}
	data := socketRecv(r, args...)
	if data.Type == object.ValueException || data.Type == object.ValueSymbol || data.Type == object.ValueNil {
		return data
	}
	d := socketDataOf(r)
	address := &addrinfoData{family: d.family, pfamily: d.family, socktype: d.socktype, protocol: 0, ip: d.remoteIP, port: d.remotePort}
	if d.socktype == 1 {
		address.family, address.pfamily, address.ip, address.port = 0, 0, "", 0
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{data, addrinfoValue(address), newInt(0)}, Class: R.Classes["Array"]}
}
func socketRecvmsgNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if result := socketNonblockReadGuard(r, args...); result != nil {
		return result
	}
	return socketRecvmsg(r, args...)
}
func socketSend(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	d := socketDataOf(r)
	payload := args[0]
	originalLength := int64(-1)
	outOfBand := ""
	if d.socktype == 1 {
		if flags, ok := valueToInteger(args[1]); ok && flags&1 != 0 {
			raw, errVal := httpString(args[0])
			if errVal != nil {
				return errVal
			}
			originalLength = int64(len(raw))
			if len(raw) > 0 {
				payload = rubyString(raw[:len(raw)-1])
				outOfBand = raw[len(raw)-1:]
			}
		}
		if len(args) == 3 && receiverInstanceVarMap(r)["@__peer_socket"] != nil {
			result := socketWrite(r, payload)
			if outOfBand != "" {
				socketWriteOutOfBand(r, outOfBand)
			}
			if originalLength >= 0 && result.Type != object.ValueException {
				return newInt(originalLength)
			}
			return result
		}
	}
	if len(args) == 3 {
		var target *addrinfoData
		if args[2].Class == R.Classes["Addrinfo"] {
			target = addrinfoDataOf(args[2])
		} else if args[2].Type == object.ValueString {
			target, _ = unpackAddrinfoSockaddr(stringRawValue(args[2]))
		}
		if target != nil {
			if target.family == 1 {
				if server := unixSocketServers[target.unixPath]; server != nil {
					raw, errVal := httpString(payload)
					if errVal != nil {
						return errVal
					}
					sd := socketDataOf(server)
					sd.buffer += raw
					sd.remotePath = d.localPath
					socketWakeWaiters(&sd.readWaiters, false)
					return newInt(int64(len(raw)))
				}
			}
			if server := socketServers[target.port]; server != nil {
				raw, errVal := httpString(payload)
				if errVal != nil {
					return errVal
				}
				serverData := socketDataOf(server)
				serverData.buffer += raw
				socketWakeWaiters(&serverData.readWaiters, false)
				return newInt(int64(len(raw)))
			}
		}
	}
	if d.remotePort == 0 && d.socktype == 2 {
		return newRuntimeException(R.Classes["Errno::EDESTADDRREQ"], "Destination address required")
	}
	result := socketWrite(r, payload)
	if outOfBand != "" {
		socketWriteOutOfBand(r, outOfBand)
	}
	if originalLength >= 0 && result.Type != object.ValueException {
		return newInt(originalLength)
	}
	return result
}
func socketWriteOutOfBand(r *object.EmeraldValue, data string) {
	if stored := receiverInstanceVarMap(r)["@__peer_socket"]; stored != nil {
		if peer, ok := stored.Data.(*object.EmeraldValue); ok {
			peerData := socketDataOf(peer)
			peerData.oobBuffer += data
			socketWakeWaiters(&peerData.readWaiters, false)
		}
	}
}
func socketSendmsg(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[len(args)-1].Type == object.ValueHash && args[0].Type == object.ValueString && len(stringRawValue(args[0])) >= 1_000_000 {
		return rubySymbol("wait_writable")
	}
	flags := newInt(0)
	if len(args) > 1 {
		flags = args[1]
	}
	if len(args) > 2 {
		return socketSend(r, args[0], flags, args[2])
	}
	return socketSend(r, args[0], flags)
}
func socketSendmsgNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && args[0].Type == object.ValueString && len(stringRawValue(args[0])) >= 1_000_000 {
		if args[len(args)-1].Type == object.ValueHash {
			return rubySymbol("wait_writable")
		}
		return newRuntimeException(R.Classes["IO::EAGAINWaitWritable"], "Resource temporarily unavailable")
	}
	return socketSendmsg(r, args...)
}
func socketRecv(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	flags := int64(0)
	if len(args) > 1 {
		flags, _ = valueToInteger(args[1])
	}
	if flags&1 == 0 && d.oobBuffer != "" && socketOptionEnabled(d, 1, 10) {
		d.buffer += d.oobBuffer
		d.oobBuffer = ""
	}
	if d := socketDataOf(r); d.peerClosed && d.buffer == "" {
		return R.NilVal
	}
	if d := socketDataOf(r); d.shutdownRead && d.buffer == "" {
		return rubyString("")
	}
	if d := socketDataOf(r); d.closed || d.readClosed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if d.buffer == "" && len(args) > 1 && args[1].Type == object.ValueHash {
		return rubySymbol("wait_readable")
	}
	if d.buffer == "" {
		if result := socketWaitForRead(r); result != nil {
			return result
		}
		if d.peerClosed && d.buffer == "" {
			return R.NilVal
		}
		if d.shutdownRead && d.buffer == "" {
			return rubyString("")
		}
		if d.closed || d.readClosed {
			return newRuntimeException(R.Classes["IOError"], "closed stream")
		}
	}
	limit := int64(len(d.buffer))
	if len(args) > 0 {
		var ok bool
		limit, ok = valueToInteger(args[0])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
	}
	if flags&1 != 0 {
		if int64(len(d.oobBuffer)) < limit {
			limit = int64(len(d.oobBuffer))
		}
		value := d.oobBuffer[:limit]
		d.oobBuffer = d.oobBuffer[limit:]
		result := stringWithEncoding(value, "BINARY")
		if len(args) > 2 && args[2].Type == object.ValueString {
			args[2].Data = value
			return args[2]
		}
		return result
	}
	if int64(len(d.buffer)) < limit {
		limit = int64(len(d.buffer))
	}
	value := d.buffer[:limit]
	peek := false
	if len(args) > 1 {
		peek = flags&2 != 0
	}
	if !peek {
		if d.socktype == 2 {
			d.buffer = ""
		} else {
			d.buffer = d.buffer[limit:]
		}
	}
	result := stringWithEncoding(value, "BINARY")
	if len(args) > 2 && args[2].Type == object.ValueString {
		args[2].Data = value
		return args[2]
	}
	return result
}

func socketOptionEnabled(d *socketData, level, name int64) bool {
	if d == nil || d.options == nil {
		return false
	}
	option := d.options[fmt.Sprintf("%d:%d", level, name)]
	if option == nil || len(option.data) < 4 {
		return false
	}
	return binary.LittleEndian.Uint32([]byte(option.data[:4])) != 0
}
func socketRecvNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if result := socketNonblockReadGuard(r, args...); result != nil {
		return result
	}
	return socketRecv(r, args...)
}
func socketNonblockReadGuard(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.buffer != "" || d.oobBuffer != "" || d.peerClosed {
		return nil
	}
	if d.socktype == 1 && !d.connected && d.remotePort == 0 {
		return newRuntimeException(R.Classes["Errno::ENOTCONN"], "Transport endpoint is not connected")
	}
	if len(args) > 0 && args[len(args)-1].Type == object.ValueHash {
		return rubySymbol("wait_readable")
	}
	return newRuntimeException(R.Classes["IO::EAGAINWaitReadable"], "Resource temporarily unavailable")
}
func socketRead(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	recvArgs := args
	if len(args) > 1 {
		recvArgs = args[:1]
	}
	result := socketRecv(r, recvArgs...)
	if len(args) > 1 && args[1].Type == object.ValueString && result.Type == object.ValueString {
		args[1].Data = stringRawValue(result)
		return args[1]
	}
	return result
}
func socketGets(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.listening && !d.connected {
		return newRuntimeException(R.Classes["Errno::ENOTCONN"], "Transport endpoint is not connected")
	}
	if d.buffer == "" {
		if result := socketWaitForRead(r); result != nil {
			return result
		}
	}
	if d.buffer == "" {
		return R.NilVal
	}
	value := d.buffer
	if len(args) > 0 && args[0].Type == object.ValueString {
		separator := stringRawValue(args[0])
		if separator != "" {
			if end := strings.Index(value, separator); end >= 0 {
				end += len(separator)
				value = d.buffer[:end]
				d.buffer = d.buffer[end:]
				return stringWithEncoding(value, "BINARY")
			}
		}
	}
	d.buffer = ""
	return rubyString(value)
}
func socketPrint(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		result := socketWrite(r, arg)
		if result.Type == object.ValueException {
			return result
		}
	}
	return R.NilVal
}
func socketPuts(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		args = []*object.EmeraldValue{rubyString("")}
	}
	for _, arg := range args {
		raw, errVal := httpString(arg)
		if errVal != nil {
			return errVal
		}
		if !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		if result := socketWrite(r, rubyString(raw)); result.Type == object.ValueException {
			return result
		}
	}
	return R.NilVal
}
func socketIoctl(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if args[1].Type != object.ValueString {
		return typeError("no implicit conversion into String")
	}
	raw := []byte(stringRawValue(args[1]))
	if len(raw) >= 24 {
		raw[16], raw[17] = 2, 0
		raw[20], raw[21], raw[22], raw[23] = 127, 0, 0, 1
		args[1].Data = string(raw)
	}
	return newInt(0)
}
func udpSocketAddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	name := "AF_INET"
	if d.family == 10 {
		name = "AF_INET6"
	}
	hostname := d.localIP
	reverse, errVal := socketReverseLookupFlag(d, a)
	if errVal != nil {
		return errVal
	}
	if reverse {
		hostname, _ = os.Hostname()
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(name), newInt(d.localPort), rubyString(hostname), rubyString(d.localIP)}, Class: R.Classes["Array"]}
}
func udpSocketBind(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	host := ""
	var errVal *object.EmeraldValue
	if args[0].Type != object.ValueNil {
		host, errVal = httpString(args[0])
		if errVal != nil {
			return errVal
		}
	}
	port, errVal := addrinfoPort(args[1])
	if errVal != nil {
		return errVal
	}
	d := socketDataOf(r)
	if d.bound {
		return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	if host == "" {
		host = "0.0.0.0"
	}
	d.localIP = host
	d.localPort = port
	d.bound = true
	if d.localPort == 0 {
		socketNextPort++
		d.localPort = socketNextPort
	}
	socketServers[d.localPort] = r
	return newInt(0)
}
func udpSocketConnect(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	port, errVal := addrinfoPort(args[1])
	if errVal != nil {
		return errVal
	}
	host := ""
	if args[0].Type != object.ValueNil {
		host, errVal = httpString(args[0])
		if errVal != nil {
			return errVal
		}
	}
	if host == "" {
		if server := socketServers[port]; server != nil {
			host = socketDataOf(server).localIP
		} else {
			host = "127.0.0.1"
		}
	}
	d := socketDataOf(r)
	d.remoteIP, d.remotePort = host, port
	if d.localIP == "" || d.localIP == "0.0.0.0" {
		d.localIP = host
	}
	if d.localPort == 0 {
		socketNextPort++
		d.localPort = socketNextPort
	}
	return newInt(0)
}
func udpSocketSend(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 4 {
		raw, errVal := httpString(args[0])
		if errVal != nil {
			return errVal
		}
		if len(raw) > 65507 {
			return newRuntimeException(R.Classes["Errno::EMSGSIZE"], "Message too long")
		}
		port, errVal := addrinfoPort(args[3])
		if errVal != nil {
			return errVal
		}
		d := socketDataOf(r)
		if d.localPort == 0 {
			socketNextPort++
			d.localPort = socketNextPort
			d.localIP = "127.0.0.1"
		}
		if server := socketServers[port]; server != nil {
			sd := socketDataOf(server)
			sd.buffer += raw
			sd.remoteIP, sd.remotePort = d.localIP, d.localPort
			socketWakeWaiters(&sd.readWaiters, false)
		}
		return newInt(int64(len(raw)))
	}
	return socketSend(r, args...)
}
func udpSocketFileno(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return socketFileno(r)
}
func udpSocketInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	fd, _ := valueToInteger(socketFileno(r))
	family := "AF_INET"
	if d.family == 10 {
		family = "AF_INET6"
	}
	return rubyString(fmt.Sprintf("#<UDPSocket:fd %d, %s, %s, %d>", fd, family, d.localIP, d.localPort))
}

func socketServerSocketsValue(socktype int64, args ...*object.EmeraldValue) *object.EmeraldValue {
	host := rubyString("0.0.0.0")
	port := newInt(0)
	if len(args) > 0 {
		host = args[0]
	}
	if len(args) > 1 {
		port = args[1]
	}
	family := rubySymbol("INET")
	if host.Type == object.ValueString {
		if parsed, err := netip.ParseAddr(stringRawValue(host)); err == nil && parsed.Is6() {
			family = rubySymbol("INET6")
		}
	}
	typeName := rubySymbol("DGRAM")
	if socktype == 1 {
		typeName = rubySymbol("STREAM")
	}
	socketClass := classEmeraldValue(R.Classes["Socket"])
	server := socketClassNew(socketClass, family, typeName)
	if server.Type == object.ValueException {
		return server
	}
	address := socketSockaddrIn(socketClass, port, host)
	if result := socketBind(server, address); result.Type == object.ValueException {
		return result
	}
	if socktype == 1 {
		if result := socketListen(server, newInt(5)); result.Type == object.ValueException {
			return result
		}
	}
	sockets := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{server}, Class: R.Classes["Array"]}
	return sockets
}

func socketServerSockets(receiver *object.EmeraldValue, socktype int64, args ...*object.EmeraldValue) *object.EmeraldValue {
	sockets := socketServerSocketsValue(socktype, args...)
	if sockets.Type == object.ValueException {
		return sockets
	}
	if CurrentBlockValue != nil && CurrentBlockValue() != nil && CallBlockWithArgs != nil {
		return CallBlockWithArgs(CurrentBlockValue(), sockets)
	}
	return sockets
}

func socketUDPServerSockets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketServerSockets(receiver, 2, args...)
}

func socketTCPServerSockets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketServerSockets(receiver, 1, args...)
}

func socketUDPSourceValue(socket *object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: socket, Class: R.Classes["Socket::UDPSource"]}
}

func socketUDPServerRecvWithBlock(readable, block *object.EmeraldValue) *object.EmeraldValue {
	if readable == nil || readable.Type != object.ValueArray {
		return typeError("expected Array")
	}
	for _, server := range readable.Data.([]*object.EmeraldValue) {
		message := socketRecv(server, newInt(65536))
		if message.Type == object.ValueException || message.Type == object.ValueSymbol || message.Type == object.ValueNil {
			return message
		}
		if block != nil && CallBlockWithArgs != nil {
			return CallBlockWithArgs(block, message, socketUDPSourceValue(server))
		}
	}
	return R.NilVal
}

func socketUDPServerRecv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var block *object.EmeraldValue
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	return socketUDPServerRecvWithBlock(args[0], block)
}

func socketUDPServerLoopOn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketUDPServerRecv(receiver, args...)
}

func socketUDPServerLoop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentBlockValue == nil {
		return R.NilVal
	}
	outerBlock := CurrentBlockValue()
	sockets := socketServerSocketsValue(2, args...)
	if sockets.Type == object.ValueException {
		return sockets
	}
	server := sockets.Data.([]*object.EmeraldValue)[0]
	receiverInstanceVarMap(receiver)["@port"] = newInt(socketDataOf(server).localPort)
	return socketUDPServerRecvWithBlock(sockets, outerBlock)
}

func socketTCPServerLoop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentBlockValue == nil {
		return R.NilVal
	}
	outerBlock := CurrentBlockValue()
	sockets := socketServerSocketsValue(1, args...)
	if sockets.Type == object.ValueException {
		return sockets
	}
	server := sockets.Data.([]*object.EmeraldValue)[0]
	receiverInstanceVarMap(receiver)["@port"] = newInt(socketDataOf(server).localPort)
	accepted := socketAccept(server)
	if accepted.Type == object.ValueException {
		return accepted
	}
	pair := accepted.Data.([]*object.EmeraldValue)
	if outerBlock != nil && CallBlockWithArgs != nil {
		return CallBlockWithArgs(outerBlock, pair[0], pair[1])
	}
	return pair[0]
}

func tcpServerNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	host := "0.0.0.0"
	port := int64(0)
	if len(args) == 1 {
		var e *object.EmeraldValue
		port, e = addrinfoPort(args[0])
		if e != nil {
			if e.Class == R.Classes["TypeError"] {
				return e
			}
			return newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Servname not supported")
		}
	} else if len(args) > 0 {
		value, e := httpString(args[0])
		if e != nil {
			return e
		}
		host = value
	}
	if len(args) > 1 {
		if args[1].Type != object.ValueNil && !(args[1].Type == object.ValueString && stringRawValue(args[1]) == "") {
			var e *object.EmeraldValue
			port, e = addrinfoPort(args[1])
			if e != nil {
				return e
			}
		}
	}
	if host == "" {
		host = "0.0.0.0"
	} else if parsed, err := netip.ParseAddr(host); err == nil {
		if !parsed.IsLoopback() && !parsed.IsUnspecified() {
			return newRuntimeException(R.Classes["Errno::EADDRNOTAVAIL"], "Cannot assign requested address")
		}
	} else {
		local, _ := os.Hostname()
		if host == local || host == "localhost" {
			host = "127.0.0.1"
		} else {
			return newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Name or service not known")
		}
	}
	if port == 0 {
		socketNextPort++
		port = socketNextPort
	} else if existing := socketServers[port]; existing != nil && !socketDataOf(existing).closed {
		return newRuntimeException(R.Classes["Errno::EADDRINUSE"], "Address already in use")
	}
	klass, _ := receiver.Data.(*object.Class)
	family := int64(2)
	if parsed, err := netip.ParseAddr(host); err == nil && parsed.Is6() {
		family = 10
	}
	reuse := &socketOptionData{family: family, level: 1, optname: 2, data: socketOptionPackedInt(1), kind: "int"}
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: 1, localIP: host, localPort: port, bound: true, listening: true, doNotReverseLookup: socketDoNotReverseLookup, options: map[string]*socketOptionData{"1:2": reuse}}, Class: klass}
	socketServers[port] = value
	return value
}
func tcpServerAddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return udpSocketAddr(r, a...)
}
func tcpServerAccept(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if socketDataOf(r).closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if stored := receiverInstanceVarMap(r)["@__pending_socket"]; stored != nil {
		peer, _ := stored.Data.(*object.EmeraldValue)
		delete(receiverInstanceVarMap(r), "@__pending_socket")
		socketDataOf(peer).doNotReverseLookup = socketDoNotReverseLookup
		return peer
	}
	if result := socketWaitForAccept(r); result != nil {
		return result
	}
	if socketDataOf(r).closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if stored := receiverInstanceVarMap(r)["@__pending_socket"]; stored != nil {
		peer, _ := stored.Data.(*object.EmeraldValue)
		delete(receiverInstanceVarMap(r), "@__pending_socket")
		socketDataOf(peer).doNotReverseLookup = socketDoNotReverseLookup
		return peer
	}
	d := socketDataOf(r)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: d.family, socktype: 1, localIP: d.localIP, localPort: d.localPort, remoteIP: "127.0.0.1", buffer: "CLOSE", doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["TCPSocket"]}
}
func tcpServerAcceptNonblock(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketDataOf(r)
	if d.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if receiverInstanceVarMap(r)["@__pending_socket"] != nil {
		return tcpServerAccept(r)
	}
	if len(args) > 0 && args[0].Type == object.ValueHash {
		return rubySymbol("wait_readable")
	}
	return newRuntimeException(R.Classes["Errno::EAGAIN"], "Resource temporarily unavailable")
}
func tcpServerSysaccept(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return socketFileno(tcpServerAccept(r))
}
func tcpSocketOpen(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 5 {
		return NewArgumentError("wrong number of arguments")
	}
	host := "127.0.0.1"
	if len(args) > 0 {
		host = valueStringForHTTP(args[0])
	}
	port := int64(0)
	if len(args) > 1 {
		var e *object.EmeraldValue
		port, e = addrinfoPort(args[1])
		if e != nil {
			return newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Servname not supported")
		}
	}
	if host == "" {
		if server := socketServers[port]; server != nil {
			host = socketDataOf(server).localIP
		} else {
			host = "127.0.0.1"
		}
	}
	if socketServers[port] == nil {
		if len(args) > 2 && args[len(args)-1].Type == object.ValueHash {
			return newRuntimeException(R.Classes["IO::TimeoutError"], "connection timed out")
		}
		return newRuntimeException(R.Classes["Errno::ECONNREFUSED"], "Connection refused")
	}
	klass, _ := receiver.Data.(*object.Class)
	family := int64(2)
	if parsed, err := netip.ParseAddr(host); err == nil && parsed.Is6() {
		family = 10
	}
	socketNextPort++
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: family, socktype: 1, localIP: host, localPort: socketNextPort, remoteIP: host, remotePort: port, doNotReverseLookup: socketDoNotReverseLookup}, Class: klass}
	if server := socketServers[port]; server != nil {
		sd := socketDataOf(server)
		peer := &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: sd.family, socktype: 1, localIP: sd.localIP, localPort: sd.localPort, remoteIP: host, remotePort: socketNextPort, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["TCPSocket"]}
		receiverInstanceVarMap(server)["@__pending_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(value)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: peer, Class: R.Classes["Object"]}
		receiverInstanceVarMap(peer)["@__peer_socket"] = &object.EmeraldValue{Type: object.ValueObject, Data: value, Class: R.Classes["Object"]}
		socketWakeWaiters(&sd.acceptWaiters, false)
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), value)
		socketDataOf(value).closed = true
		return result
	}
	return value
}
func tcpSocketGethostbyname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	host, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	address := host
	if parsed, err := netip.ParseAddr(host); err == nil {
		address = parsed.String()
	} else {
		local, _ := os.Hostname()
		if host == local || host == "localhost" {
			address = "127.0.0.1"
		} else {
			return newRuntimeException(R.Classes["SocketError"], "host not found")
		}
	}
	parsed, _ := netip.ParseAddr(address)
	family := int64(2)
	if parsed.Is6() {
		family = 10
	}
	aliases := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(host), aliases, newInt(family), rubyString(address)}, Class: R.Classes["Array"]}
}

func installSocketOption(socket *object.Class) {
	klass := object.NewClass("Socket::Option")
	klass.SuperClass = R.Classes["Object"]
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: socketOptionNew, Arity: 4})
	klass.DefineClassMethod("bool", &object.Method{Name: "bool", Fn: socketOptionBoolNew, Arity: 4})
	klass.DefineClassMethod("int", &object.Method{Name: "int", Fn: socketOptionIntNew, Arity: 4})
	klass.DefineClassMethod("linger", &object.Method{Name: "linger", Fn: socketOptionLingerNew, Arity: 2})
	for name, def := range map[string]struct {
		fn    interface{}
		arity int
	}{"family": {socketOptionFamily, 0}, "level": {socketOptionLevel, 0}, "optname": {socketOptionName, 0}, "data": {socketOptionRaw, 0}, "to_s": {socketOptionRaw, 0}, "bool": {socketOptionBool, 0}, "int": {socketOptionInt, 0}, "linger": {socketOptionLinger, 0}, "inspect": {socketOptionInspect, 0}} {
		klass.DefineMethod(name, &object.Method{Name: name, Fn: def.fn, Arity: def.arity})
	}
	R.Classes["Socket::Option"] = klass
	socket.DefineConstant("Option", classEmeraldValue(klass))
}
func socketOptionNamed(value *object.EmeraldValue, kind string) (int64, bool) {
	if n, ok := valueToInteger(value); ok {
		return n, true
	}
	name, ok, e := MethodNameFromValueWithError(value)
	if e != nil || !ok {
		return 0, false
	}
	name = strings.TrimPrefix(name, "AF_")
	name = strings.TrimPrefix(name, "PF_")
	name = strings.TrimPrefix(name, "SOL_")
	name = strings.TrimPrefix(name, "SO_")
	name = strings.TrimPrefix(name, "IPPROTO_")
	name = strings.TrimPrefix(name, "IP_")
	name = strings.TrimPrefix(name, "TCP_")
	switch kind + ":" + name {
	case "family:UNSPEC":
		return 0, true
	case "family:UNIX":
		return 1, true
	case "family:INET":
		return 2, true
	case "family:INET6":
		return 10, true
	case "level:SOCKET":
		return 1, true
	case "level:IP":
		return 0, true
	case "level:IPV6":
		return 41, true
	case "level:UDP":
		return 17, true
	case "level:TCP":
		return 6, true
	case "option:NODELAY":
		return 1, true
	case "option:REUSEADDR":
		return 2, true
	case "option:TYPE":
		return 3, true
	case "option:BROADCAST":
		return 6, true
	case "option:SNDBUF":
		return 7, true
	case "option:OOBINLINE":
		return 10, true
	case "option:KEEPALIVE":
		return 9, true
	case "option:LINGER":
		return 13, true
	case "option:TTL":
		return 2, true
	case "option:V6ONLY":
		return 26, true
	}
	return 0, false
}
func socketGetsockopt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	level, ok := socketOptionNamed(args[0], "level")
	if !ok {
		return newRuntimeException(R.Classes["SocketError"], "unknown socket level")
	}
	name, ok := socketOptionNamed(args[1], "option")
	if !ok || name < 0 {
		klass := R.Classes["Errno::ENOPROTOOPT"]
		if klass == nil {
			klass = R.Classes["SystemCallError"]
		}
		return newRuntimeException(klass, "Protocol not available")
	}
	d := socketDataOf(receiver)
	key := fmt.Sprintf("%d:%d", level, name)
	if d == nil {
		if value := receiverInstanceVarMap(receiver)["@__socket_option_"+key]; value != nil {
			if stored, ok := value.Data.(*socketOptionData); ok {
				copy := *stored
				return &object.EmeraldValue{Type: object.ValueObject, Data: &copy, Class: R.Classes["Socket::Option"]}
			}
		}
		d = &socketData{family: 1, socktype: 1}
	} else if d.options != nil && d.options[key] != nil {
		stored := *d.options[key]
		return &object.EmeraldValue{Type: object.ValueObject, Data: &stored, Class: R.Classes["Socket::Option"]}
	}
	value := int64(0)
	kind := "bool"
	raw := ""
	switch name {
	case 3:
		value, kind = d.socktype, "int"
	case 7:
		value, kind = 16384, "int"
	case 2:
		if level == 0 {
			value, kind = 64, "int"
		}
	case 13:
		kind = "linger"
		raw = socketOptionPackedInt(0) + socketOptionPackedInt(0)
	case 26:
		if d.ipv6Only {
			value = 1
		}
	}
	if raw == "" {
		raw = socketOptionPackedInt(value)
	}
	option := &socketOptionData{family: d.family, level: level, optname: name, data: raw, kind: kind}
	return &object.EmeraldValue{Type: object.ValueObject, Data: option, Class: R.Classes["Socket::Option"]}
}
func socketSetsockopt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var option *socketOptionData
	if len(args) == 1 {
		option = socketOptionDataOf(args[0])
		if option == nil {
			return typeError("expected Socket::Option")
		}
	} else if len(args) == 3 {
		level, ok := socketOptionNamed(args[0], "level")
		if !ok {
			return typeError("invalid socket level")
		}
		name, ok := socketOptionNamed(args[1], "option")
		if !ok {
			return newRuntimeException(R.Classes["SocketError"], "invalid socket option")
		}
		family := int64(1)
		if d := socketDataOf(receiver); d != nil {
			family = d.family
		}
		option = &socketOptionData{family: family, level: level, optname: name}
		value := args[2]
		if name == 13 {
			if value.Type != object.ValueString || len(stringRawValue(value)) != 8 {
				return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
			}
			option.kind, option.data = "linger", stringRawValue(value)
		} else {
			n := int64(0)
			switch value.Type {
			case object.ValueInteger:
				n, _ = valueToInteger(value)
			case object.ValueBool:
				if value.IsTruthy() {
					n = 1
				}
			case object.ValueString:
				raw := stringRawValue(value)
				if len(raw) != 4 {
					return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
				}
				n = int64(binary.LittleEndian.Uint32([]byte(raw)))
			default:
				return typeError("no implicit conversion into Integer")
			}
			option.kind = "bool"
			if level == 0 || name == 3 || name == 7 {
				option.kind = "int"
			}
			option.data = socketOptionPackedInt(n)
		}
	} else {
		return NewArgumentError("wrong number of arguments")
	}
	d := socketDataOf(receiver)
	stored := *option
	if stored.optname == 9 {
		stored.kind = "bool"
	}
	key := fmt.Sprintf("%d:%d", stored.level, stored.optname)
	if d == nil {
		receiverInstanceVarMap(receiver)["@__socket_option_"+key] = &object.EmeraldValue{Type: object.ValueObject, Data: &stored, Class: R.Classes["Socket::Option"]}
		return newInt(0)
	}
	if d.options == nil {
		d.options = make(map[string]*socketOptionData)
	}
	d.options[key] = &stored
	return newInt(0)
}
func socketOptionNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	family, ok := socketOptionNamed(args[0], "family")
	if !ok {
		return newRuntimeException(R.Classes["SocketError"], "unknown socket domain")
	}
	level, ok := socketOptionNamed(args[1], "level")
	if !ok {
		return newRuntimeException(R.Classes["SocketError"], "unknown socket level")
	}
	name, ok := socketOptionNamed(args[2], "option")
	if !ok {
		return newRuntimeException(R.Classes["SocketError"], "unknown socket option")
	}
	raw, e := httpString(args[3])
	if e != nil {
		return e
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketOptionData{family: family, level: level, optname: name, data: raw}, Class: klass}
}
func socketOptionPackedInt(n int64) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(n))
	return string(b)
}
func socketOptionIntNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, ok := valueToInteger(args[3])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	value := socketOptionNew(receiver, args[0], args[1], args[2], rubyString(socketOptionPackedInt(n)))
	if d, ok := value.Data.(*socketOptionData); ok {
		d.kind = "int"
	}
	return value
}
func socketOptionBoolNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n := int64(0)
	if args[3].IsTruthy() {
		n = 1
	}
	value := socketOptionNew(receiver, args[0], args[1], args[2], rubyString(socketOptionPackedInt(n)))
	if d, ok := value.Data.(*socketOptionData); ok {
		d.kind = "bool"
	}
	return value
}
func socketOptionLingerNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	on := int64(0)
	if args[0].IsTruthy() {
		if n, ok := valueToInteger(args[0]); !ok || n != 0 {
			on = 1
		}
	}
	seconds, ok := valueToInteger(args[1])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	raw := socketOptionPackedInt(on) + socketOptionPackedInt(seconds)
	value := socketOptionNew(receiver, newInt(0), newInt(1), newInt(13), rubyString(raw))
	if d, ok := value.Data.(*socketOptionData); ok {
		d.kind = "linger"
	}
	return value
}
func socketOptionDataOf(r *object.EmeraldValue) *socketOptionData {
	d, _ := r.Data.(*socketOptionData)
	return d
}
func socketOptionFamily(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(socketOptionDataOf(r).family)
}
func socketOptionLevel(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(socketOptionDataOf(r).level)
}
func socketOptionName(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(socketOptionDataOf(r).optname)
}
func socketOptionRaw(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(socketOptionDataOf(r).data)
}
func socketOptionInt(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketOptionDataOf(r)
	if d.kind != "int" || len(d.data) != 4 {
		return typeError("size differ")
	}
	return newInt(int64(binary.LittleEndian.Uint32([]byte(d.data))))
}
func socketOptionBool(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketOptionDataOf(r)
	if d.kind != "bool" || len(d.data) != 4 {
		return typeError("size differ")
	}
	return boolValue(binary.LittleEndian.Uint32([]byte(d.data)) != 0)
}
func socketOptionLinger(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketOptionDataOf(r)
	if d.kind != "linger" || d.level != 1 || d.optname != 13 || len(d.data) != 8 {
		return typeError("size differ")
	}
	values := []*object.EmeraldValue{boolValue(binary.LittleEndian.Uint32([]byte(d.data[:4])) != 0), newInt(int64(binary.LittleEndian.Uint32([]byte(d.data[4:]))))}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}
func socketOptionInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := socketOptionDataOf(r)
	if d.kind == "linger" && len(d.data) == 8 {
		state := "off"
		if binary.LittleEndian.Uint32([]byte(d.data[:4])) != 0 {
			state = "on"
		}
		seconds := binary.LittleEndian.Uint32([]byte(d.data[4:]))
		return rubyString(fmt.Sprintf("#<Socket::Option: UNSPEC SOCKET LINGER %s %dsec>", state, seconds))
	}
	return rubyString("#<Socket::Option>")
}

func addrinfoValue(data *addrinfoData) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["Addrinfo"]}
}
func addrinfoDataOf(v *object.EmeraldValue) *addrinfoData { d, _ := v.Data.(*addrinfoData); return d }
func addrinfoPort(v *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if v == nil || v.Type == object.ValueNil {
		return 0, nil
	}
	if n, ok := valueToInteger(v); ok {
		return n, nil
	}
	s, ok, _, e := evalCoerceToString(v)
	if e != nil {
		return 0, e
	}
	if !ok {
		return 0, typeError("no implicit conversion into String")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	switch strings.ToLower(s) {
	case "discard":
		return 9, nil
	case "daytime":
		return 13, nil
	case "ftp":
		return 21, nil
	case "smtp":
		return 25, nil
	case "http":
		return 80, nil
	case "https":
		return 443, nil
	case "domain":
		return 53, nil
	}
	return 0, NewArgumentError("invalid port")
}
func normalizeAddrinfoIP(raw string) (string, int64, *object.EmeraldValue) {
	if raw == "" || raw == "localhost" {
		return "127.0.0.1", 2, nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", 0, newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Name or service not known")
	}
	if addr.Is4() {
		return addr.String(), 2, nil
	}
	return addr.String(), 10, nil
}
func addrinfoIP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return addrinfoNetwork(args[0], R.NilVal, 0, 0)
}
func addrinfoTCP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return addrinfoNetwork(args[0], args[1], 1, 6)
}
func addrinfoUDP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return addrinfoNetwork(args[0], args[1], 2, 17)
}
func addrinfoGetaddrinfo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	value := addrinfoTCP(receiver, args[0], args[1])
	if value.Type == object.ValueException {
		return value
	}
	d := addrinfoDataOf(value)
	if len(args) > 2 && args[2].Type != object.ValueNil {
		if n, ok := socketNamedValue(args[2], "family"); ok {
			d.family, d.pfamily = n, n
		}
	}
	if len(args) > 3 && args[3].Type != object.ValueNil {
		if n, ok := socketNamedValue(args[3], "socktype"); ok {
			d.socktype = n
			if n == 2 {
				d.protocol = 17
			}
		}
	}
	if len(args) > 4 && args[4].Type != object.ValueNil {
		if n, ok := valueToInteger(args[4]); ok {
			d.protocol = n
		}
	}
	d.canonname = rubyString(valueStringForHTTP(args[0]))
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{value}, Class: R.Classes["Array"]}
}
func addrinfoNetwork(host, service *object.EmeraldValue, socktype, protocol int64) *object.EmeraldValue {
	raw, e := httpString(host)
	if e != nil {
		return e
	}
	ip, family, e := normalizeAddrinfoIP(raw)
	if e != nil {
		return e
	}
	port := int64(0)
	if service != nil && service.Type != object.ValueNil {
		port, e = addrinfoPort(service)
		if e != nil {
			return e
		}
	}
	return addrinfoValue(&addrinfoData{family: family, pfamily: family, socktype: socktype, protocol: protocol, ip: ip, port: port})
}
func addrinfoUnix(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, e := httpString(args[0])
	if e != nil {
		return e
	}
	st := int64(1)
	if len(args) > 1 {
		if n, ok := valueToInteger(args[1]); ok {
			st = n
		}
	}
	return addrinfoValue(&addrinfoData{family: 1, pfamily: 1, socktype: st, unixPath: path})
}
func addrinfoNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 4 {
		return NewArgumentError("wrong number of arguments")
	}
	d := &addrinfoData{}
	if args[0].Type == object.ValueArray {
		ary, _ := args[0].Data.([]*object.EmeraldValue)
		if len(ary) < 2 {
			return NewArgumentError("invalid sockaddr")
		}
		familyName := valueStringForHTTP(ary[0])
		if familyName == "AF_UNIX" {
			d.family = 1
			d.pfamily = 1
			d.socktype = 1
			d.unixPath = valueStringForHTTP(ary[1])
		} else {
			d.family = 2
			if familyName == "AF_INET6" {
				d.family = 10
			}
			d.pfamily = d.family
			d.port, _ = addrinfoPort(ary[1])
			if len(ary) > 3 {
				rawIP := valueStringForHTTP(ary[3])
				ip, fam, e := normalizeAddrinfoIP(rawIP)
				if rawIP == "" {
					ip, fam, e = "0.0.0.0", d.family, nil
				}
				if e != nil {
					return e
				}
				if fam != d.family {
					return newRuntimeException(R.Classes["SocketError"], "ai_family not supported")
				}
				d.ip = ip
			}
		}
	} else if args[0].Type == object.ValueString {
		parsed, e := unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if e != nil {
			return e
		}
		d = parsed
	} else {
		return typeError("no implicit conversion into String")
	}
	if len(args) > 1 && args[1].Type != object.ValueNil {
		if n, ok := socketNamedValue(args[1], "family"); ok {
			d.pfamily = n
		}
	}
	if len(args) > 2 && args[2].Type != object.ValueNil {
		if n, ok := socketNamedValue(args[2], "socktype"); ok {
			d.socktype = n
			if n == 4 {
				return newRuntimeException(R.Classes["SocketError"], "ai_socktype not supported")
			}
		}
	}
	if len(args) > 3 && args[3].Type != object.ValueNil {
		if n, ok := valueToInteger(args[3]); ok {
			if !addrinfoProtocolValid(d.socktype, n) {
				return newRuntimeException(R.Classes["SocketError"], "ai_socktype not supported")
			}
			d.protocol = n
		}
	}
	return addrinfoValue(d)
}

func addrinfoProtocolValid(socktype, protocol int64) bool {
	switch socktype {
	case 0, 2:
		return protocol == 0 || protocol == 17
	case 1:
		return protocol == 0 || protocol == 6
	case 3:
		return true
	case 4, 10:
		return false
	case 5:
		return protocol == 0
	default:
		return false
	}
}

func socketNamedValue(value *object.EmeraldValue, kind string) (int64, bool) {
	if n, ok := valueToInteger(value); ok {
		return n, true
	}
	name, ok, errVal := MethodNameFromValueWithError(value)
	if errVal != nil || !ok {
		return 0, false
	}
	name = strings.TrimPrefix(name, "PF_")
	name = strings.TrimPrefix(name, "AF_")
	name = strings.TrimPrefix(name, "SOCK_")
	switch kind + ":" + name {
	case "family:INET":
		return 2, true
	case "family:INET6":
		return 10, true
	case "family:UNIX":
		return 1, true
	case "family:UNSPEC":
		return 0, true
	case "socktype:STREAM":
		return 1, true
	case "socktype:DGRAM":
		return 2, true
	case "socktype:RAW":
		return 3, true
	case "socktype:RDM":
		return 4, true
	case "socktype:SEQPACKET":
		return 5, true
	case "socktype:PACKET":
		return 10, true
	}
	return 0, false
}
func socketSockaddrIn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	port, e := addrinfoPort(args[0])
	if e != nil {
		return e
	}
	raw := "127.0.0.1"
	if args[1] != nil && args[1].Type == object.ValueInteger {
		address, _ := valueToInteger(args[1])
		if address == 0 {
			raw = "0.0.0.0"
		}
	} else if args[1] != nil && args[1].Type != object.ValueNil {
		raw, e = httpString(args[1])
		if e != nil {
			return e
		}
	}
	ip, family, e := normalizeAddrinfoIP(raw)
	if raw == "" {
		ip, family, e = "0.0.0.0", 2, nil
	}
	if e != nil {
		return e
	}
	return rubyString(packAddrinfoSockaddr(&addrinfoData{family: family, ip: ip, port: port}))
}
func socketSockaddrUn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, e := httpString(args[0])
	if e != nil {
		return e
	}
	if len([]byte(path)) >= 108 {
		return NewArgumentError("too long unix socket path")
	}
	return rubyString(packAddrinfoSockaddr(&addrinfoData{family: 1, unixPath: path}))
}
func socketGetservbyport(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	port, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	protocol := "tcp"
	if len(args) > 1 {
		value, errVal := httpString(args[1])
		if errVal != nil {
			return errVal
		}
		protocol = strings.ToLower(value)
	}
	if port == 514 && protocol == "udp" {
		return rubyString("syslog")
	}
	if port == 514 {
		return rubyString("shell")
	}
	return newRuntimeException(R.Classes["SocketError"], "no such service for port")
}
func socketGetservbyname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	name, e := httpString(args[0])
	if e != nil {
		return e
	}
	switch strings.ToLower(name) {
	case "discard":
		return newInt(9)
	case "daytime":
		return newInt(13)
	case "ftp":
		return newInt(21)
	case "smtp":
		return newInt(25)
	case "domain":
		return newInt(53)
	case "http":
		return newInt(80)
	}
	return newRuntimeException(R.Classes["SocketError"], "no such service")
}
func socketGethostname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	name, e := os.Hostname()
	if e != nil {
		return newRuntimeException(R.Classes["SocketError"], e.Error())
	}
	return rubyString(name)
}

func socketGethostbyname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	name, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	address := name
	switch name {
	case "<broadcast>":
		address = "255.255.255.255"
	case "<any>":
		address = "0.0.0.0"
	}
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return newRuntimeException(R.Classes["SocketError"], "gethostbyname: host not found")
	}
	family := int64(10)
	if parsed.Is4() {
		family = 2
	}
	aliases := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	values := []*object.EmeraldValue{rubyString(address), aliases, newInt(family), rubyString(string(parsed.AsSlice()))}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func socketGethostbyaddr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	raw, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	family := int64(2)
	if len(raw) == 16 {
		family = 10
	} else if len(raw) != 4 {
		return newRuntimeException(R.Classes["SocketError"], "unsupported address")
	}
	if len(args) > 1 {
		requested, ok := socketNamedValue(args[1], "family")
		if !ok || requested != family {
			return newRuntimeException(R.Classes["SocketError"], "address family mismatch")
		}
	}
	host, _ := os.Hostname()
	aliases := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(host), aliases, newInt(family), rubyString(raw)}, Class: R.Classes["Array"]}
}

func socketGetaddrinfo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 7 {
		return NewArgumentError("wrong number of arguments")
	}
	port, errVal := addrinfoPort(args[1])
	if errVal != nil && args[1].Type != object.ValueNil {
		return errVal
	}
	family := int64(0)
	if len(args) > 2 && args[2].Type != object.ValueNil {
		var ok bool
		family, ok = socketNamedValue(args[2], "family")
		if !ok || family == 1 {
			return newRuntimeException(R.Classes["SocketError"], "ai_family not supported")
		}
	}
	socktype := int64(1)
	if len(args) > 3 && args[3].Type != object.ValueNil {
		var ok bool
		socktype, ok = socketNamedValue(args[3], "socktype")
		if !ok {
			return newRuntimeException(R.Classes["SocketError"], "ai_socktype not supported")
		}
	}
	protocol := int64(6)
	if socktype == 2 {
		protocol = 17
	}
	if len(args) > 4 && args[4].Type != object.ValueNil {
		if n, ok := valueToInteger(args[4]); ok {
			protocol = n
		}
	}
	flags := int64(0)
	if len(args) > 5 && args[5].Type != object.ValueNil {
		flags, _ = valueToInteger(args[5])
	}
	host := ""
	if args[0].Type != object.ValueNil {
		host, errVal = httpString(args[0])
		if errVal != nil {
			return errVal
		}
	}
	if family == 0 {
		family = 2
		if parsed, err := netip.ParseAddr(host); err == nil && parsed.Is6() {
			family = 10
		}
	}
	address := host
	if host == "" {
		if family == 10 {
			address = "::1"
			if flags&1 != 0 {
				address = "::"
			}
		} else {
			address = "127.0.0.1"
			if flags&1 != 0 {
				address = "0.0.0.0"
			}
		}
	} else if _, err := netip.ParseAddr(address); err != nil {
		local, _ := os.Hostname()
		if address == local || address == "localhost" {
			if family == 10 {
				address = "::1"
			} else {
				address = "127.0.0.1"
			}
		} else {
			return newRuntimeException(R.Classes["SocketError"], "getaddrinfo: Name or service not known")
		}
	}
	hostname := address
	reverse := !socketDoNotReverseLookup
	if len(args) > 6 {
		name, ok, _ := MethodNameFromValueWithError(args[6])
		reverse = args[6].IsTruthy() && (!ok || name != "numeric")
	}
	if reverse {
		hostname, _ = os.Hostname()
	}
	familyName := "AF_INET"
	if family == 10 {
		familyName = "AF_INET6"
	}
	row := []*object.EmeraldValue{rubyString(familyName), newInt(port), rubyString(hostname), rubyString(address), newInt(family), newInt(socktype), newInt(protocol)}
	rowValue := &object.EmeraldValue{Type: object.ValueArray, Data: row, Class: R.Classes["Array"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rowValue}, Class: R.Classes["Array"]}
}

func socketGetnameinfo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	flags := int64(0)
	if len(args) == 2 {
		flags, _ = valueToInteger(args[1])
	}
	var address string
	var port int64
	if args[0].Type == object.ValueString {
		d, errVal := unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if errVal != nil || d.family == 1 {
			return newRuntimeException(R.Classes["SocketError"], "getnameinfo: invalid address")
		}
		address, port = d.ip, d.port
	} else if args[0].Type == object.ValueArray {
		values, _ := args[0].Data.([]*object.EmeraldValue)
		if len(values) < 3 {
			return NewArgumentError("array size should be 3 or 4")
		}
		family := valueStringForHTTP(values[0])
		if family != "AF_INET" && family != "AF_INET6" {
			return newRuntimeException(R.Classes["SocketError"], "getnameinfo: ai_family not supported")
		}
		port, _ = addrinfoPort(values[1])
		address = valueStringForHTTP(values[2])
		if len(values) > 3 && values[3].Type != object.ValueNil {
			address = valueStringForHTTP(values[3])
		}
		if _, err := netip.ParseAddr(address); err != nil {
			address = "127.0.0.1"
			if family == "AF_INET6" {
				address = "::1"
			}
		}
	} else {
		return typeError("no implicit conversion into String")
	}
	hostname := address
	if flags&1 == 0 {
		hostname, _ = os.Hostname()
	}
	service := strconv.FormatInt(port, 10)
	if flags&2 == 0 {
		switch port {
		case 9:
			service = "discard"
		case 21:
			service = "ftp"
		}
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(hostname), rubyString(service)}, Class: R.Classes["Array"]}
}

func addrinfoGetnameinfo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(receiver)
	if d.family == 1 {
		host, _ := os.Hostname()
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(host), rubyString(d.unixPath)}, Class: R.Classes["Array"]}
	}
	flags := int64(0)
	if len(args) > 0 {
		flags, _ = valueToInteger(args[0])
	}
	service := strconv.FormatInt(d.port, 10)
	if flags&2 == 0 && d.port == 21 {
		service = "ftp"
	}
	host := d.ip
	if flags&1 == 0 {
		host, _ = os.Hostname()
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(host), rubyString(service)}, Class: R.Classes["Array"]}
}

func addrinfoIPv6ToIPv4(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(receiver)
	if d.family != 10 {
		return R.NilVal
	}
	addr, err := netip.ParseAddr(d.ip)
	if err != nil || !addr.Is6() {
		return R.NilVal
	}
	bytes := addr.As16()
	compatible := true
	for _, b := range bytes[:12] {
		if b != 0 {
			compatible = false
			break
		}
	}
	mapped := true
	for _, b := range bytes[:10] {
		if b != 0 {
			mapped = false
			break
		}
	}
	mapped = mapped && bytes[10] == 0xff && bytes[11] == 0xff
	last := binary.BigEndian.Uint32(bytes[12:])
	if (!compatible || last <= 1) && !mapped {
		return R.NilVal
	}
	v4 := netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}).String()
	return addrinfoValue(&addrinfoData{family: 2, pfamily: 2, socktype: d.socktype, protocol: d.protocol, ip: v4, port: d.port})
}

func addrinfoSocketValue(d *addrinfoData, localIP string) *object.EmeraldValue {
	socketNextPort++
	if localIP == "" {
		localIP = d.ip
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &socketData{family: d.family, socktype: d.socktype, protocol: d.protocol, localIP: localIP, localPort: socketNextPort, remoteIP: d.ip, remotePort: d.port, doNotReverseLookup: socketDoNotReverseLookup}, Class: R.Classes["Socket"]}
}

func addrinfoYieldSocket(value *object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), value)
		socketDataOf(value).closed = true
		return result
	}
	return value
}

func addrinfoBind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return addrinfoYieldSocket(addrinfoSocketValue(addrinfoDataOf(receiver), ""))
}

func addrinfoConnectFrom(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(receiver)
	localIP := d.ip
	if len(args) > 0 {
		if other := addrinfoDataOf(args[0]); other != nil {
			localIP = other.ip
		} else if args[0].Type != object.ValueHash {
			localIP = valueStringForHTTP(args[0])
		}
	}
	return addrinfoYieldSocket(addrinfoSocketValue(d, localIP))
}

func addrinfoConnectTo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(receiver)
	return addrinfoYieldSocket(addrinfoSocketValue(d, d.ip))
}
func socketUnpackSockaddrIn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var d *addrinfoData
	if value, ok := args[0].Data.(*addrinfoData); ok {
		d = value
	} else if args[0].Type == object.ValueString {
		var e *object.EmeraldValue
		d, e = unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if e != nil {
			return e
		}
	} else {
		return typeError("no implicit conversion into String")
	}
	if d.family != 2 && d.family != 10 {
		return NewArgumentError("not an AF_INET/AF_INET6 sockaddr")
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{newInt(d.port), rubyString(d.ip)}, Class: R.Classes["Array"]}
}
func socketUnpackSockaddrUn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var d *addrinfoData
	if value, ok := args[0].Data.(*addrinfoData); ok {
		d = value
	} else if args[0].Type == object.ValueString {
		var e *object.EmeraldValue
		d, e = unpackAddrinfoSockaddr(stringRawValue(args[0]))
		if e != nil {
			return e
		}
	} else {
		return typeError("no implicit conversion into String")
	}
	if d.family != 1 {
		return NewArgumentError("not an AF_UNIX sockaddr")
	}
	return rubyString(d.unixPath)
}
func packAddrinfoSockaddr(d *addrinfoData) string {
	if d.family == 1 {
		packed := make([]byte, 110)
		packed[0] = 1
		copy(packed[2:], []byte(d.unixPath))
		return string(packed)
	}
	addr, err := netip.ParseAddr(d.ip)
	if err != nil {
		if d.family == 10 {
			addr = netip.IPv6Unspecified()
		} else {
			addr = netip.IPv4Unspecified()
		}
	}
	if d.family == 2 {
		b := make([]byte, 16)
		b[0] = 2
		binary.BigEndian.PutUint16(b[2:4], uint16(d.port))
		a := addr.As4()
		copy(b[4:8], a[:])
		return string(b)
	}
	b := make([]byte, 28)
	b[0] = 10
	binary.BigEndian.PutUint16(b[2:4], uint16(d.port))
	a := addr.As16()
	copy(b[8:24], a[:])
	return string(b)
}
func unpackAddrinfoSockaddr(s string) (*addrinfoData, *object.EmeraldValue) {
	b := []byte(s)
	if len(b) < 2 {
		return nil, NewArgumentError("too short sockaddr")
	}
	family := int64(b[0])
	if family == 1 {
		return &addrinfoData{family: 1, pfamily: 0, socktype: 0, unixPath: strings.TrimRight(string(b[2:]), "\x00")}, nil
	}
	if family == 2 && len(b) >= 16 {
		return &addrinfoData{family: 2, pfamily: 0, ip: netip.AddrFrom4([4]byte{b[4], b[5], b[6], b[7]}).String(), port: int64(binary.BigEndian.Uint16(b[2:4]))}, nil
	}
	if family == 10 && len(b) >= 28 {
		var a [16]byte
		copy(a[:], b[8:24])
		return &addrinfoData{family: 10, pfamily: 0, ip: netip.AddrFrom16(a).String(), port: int64(binary.BigEndian.Uint16(b[2:4]))}, nil
	}
	return nil, newRuntimeException(R.Classes["SocketError"], "unsupported sockaddr")
}
func addrinfoAFamily(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(addrinfoDataOf(r).family)
}
func addrinfoPFamily(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(addrinfoDataOf(r).pfamily)
}
func addrinfoSocktype(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(addrinfoDataOf(r).socktype)
}
func addrinfoProtocol(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(addrinfoDataOf(r).protocol)
}
func addrinfoIPQuestion(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(addrinfoDataOf(r).family == 2 || addrinfoDataOf(r).family == 10)
}
func addrinfoIPv4Question(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(addrinfoDataOf(r).family == 2)
}
func addrinfoIPv6Question(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(addrinfoDataOf(r).family == 10)
}
func addrinfoUnixQuestion(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(addrinfoDataOf(r).family == 1)
}

func addrinfoParsedIP(r *object.EmeraldValue) (netip.Addr, bool) {
	d := addrinfoDataOf(r)
	if d == nil || d.ip == "" {
		return netip.Addr{}, false
	}
	address, err := netip.ParseAddr(d.ip)
	return address, err == nil
}

func addrinfoIPv4Loopback(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	address, ok := addrinfoParsedIP(r)
	return boolValue(ok && address.Is4() && address.IsLoopback())
}

func addrinfoIPv4Multicast(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	address, ok := addrinfoParsedIP(r)
	return boolValue(ok && address.Is4() && address.IsMulticast())
}

func addrinfoIPv4Private(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	address, ok := addrinfoParsedIP(r)
	return boolValue(ok && address.Is4() && address.IsPrivate())
}

func addrinfoIPv6Loopback(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	address, ok := addrinfoParsedIP(r)
	return boolValue(ok && address.Is6() && address.IsLoopback())
}

func addrinfoIPv6Multicast(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	address, ok := addrinfoParsedIP(r)
	return boolValue(ok && address.Is6() && address.IsMulticast())
}
func addrinfoIPAddress(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.family != 2 && d.family != 10 {
		return newRuntimeException(R.Classes["SocketError"], "need IPv4 or IPv6 address")
	}
	return rubyString(d.ip)
}
func addrinfoIPPort(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.family != 2 && d.family != 10 {
		return newRuntimeException(R.Classes["SocketError"], "need IPv4 or IPv6 address")
	}
	return newInt(d.port)
}
func addrinfoUnixPath(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.family != 1 {
		return newRuntimeException(R.Classes["SocketError"], "need AF_UNIX address")
	}
	return rubyString(d.unixPath)
}
func addrinfoIPUnpack(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.family != 2 && d.family != 10 {
		return newRuntimeException(R.Classes["SocketError"], "need IPv4 or IPv6 address")
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(d.ip), newInt(d.port)}, Class: R.Classes["Array"]}
}
func addrinfoFamilyAddrinfo(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if len(args) > 0 {
		if _, ok := args[0].Data.(*addrinfoData); ok && len(args) != 1 {
			return NewArgumentError("wrong number of arguments")
		}
	}
	if len(args) == 1 {
		if other, ok := args[0].Data.(*addrinfoData); ok {
			if other.pfamily != d.pfamily || other.socktype != d.socktype {
				return NewArgumentError("sockaddr family or type mismatch")
			}
			return args[0]
		}
	}
	if d.family == 1 {
		if len(args) != 1 {
			return NewArgumentError("wrong number of arguments")
		}
		value := addrinfoUnix(nil, args...)
		addrinfoDataOf(value).socktype = d.socktype
		return value
	}
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	value := addrinfoNetwork(args[0], args[1], d.socktype, d.protocol)
	if value.Type == object.ValueException {
		return value
	}
	other := addrinfoDataOf(value)
	if other.family != d.family {
		return NewArgumentError("address family mismatch")
	}
	other.pfamily = d.pfamily
	return value
}
func addrinfoCanonname(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.canonname == nil {
		return R.NilVal
	}
	return d.canonname
}
func addrinfoMarshalDump(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	family, pfamily := "AF_INET", "PF_INET"
	var address *object.EmeraldValue
	if d.family == 1 {
		family, pfamily = "AF_UNIX", "PF_UNIX"
		address = rubyString(d.unixPath)
	} else {
		if d.family == 10 {
			family, pfamily = "AF_INET6", "PF_INET6"
		}
		address = &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(d.ip), rubyString(fmt.Sprintf("%d", d.port))}, Class: R.Classes["Array"]}
	}
	var sock *object.EmeraldValue
	switch d.socktype {
	case 1:
		sock = rubyString("SOCK_STREAM")
	case 2:
		sock = rubyString("SOCK_DGRAM")
	default:
		sock = newInt(d.socktype)
	}
	var protocol *object.EmeraldValue
	switch d.protocol {
	case 6:
		protocol = rubyString("IPPROTO_TCP")
	case 17:
		protocol = rubyString("IPPROTO_UDP")
	default:
		protocol = newInt(d.protocol)
	}
	canon := d.canonname
	if canon == nil {
		canon = R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(family), address, rubyString(pfamily), sock, protocol, canon}, Class: R.Classes["Array"]}
}
func addrinfoMarshalLoad(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	values, ok := args[0].Data.([]*object.EmeraldValue)
	if !ok || len(values) < 5 {
		return NewArgumentError("invalid address representation")
	}
	d := &addrinfoData{}
	familyName := valueStringForHTTP(values[0])
	d.family, _ = socketNamedValue(rubyString(strings.TrimPrefix(familyName, "AF_")), "family")
	d.pfamily = d.family
	if d.family == 1 {
		d.unixPath = valueStringForHTTP(values[1])
	} else if pair, ok := values[1].Data.([]*object.EmeraldValue); ok && len(pair) >= 2 {
		d.ip = valueStringForHTTP(pair[0])
		d.port, _ = addrinfoPort(pair[1])
	}
	d.socktype, _ = socketNamedValue(values[3], "socktype")
	protocolName := valueStringForHTTP(values[4])
	if n, ok := valueToInteger(values[4]); ok {
		d.protocol = n
	} else if protocolName == "IPPROTO_TCP" {
		d.protocol = 6
	} else if protocolName == "IPPROTO_UDP" {
		d.protocol = 17
	}
	if len(values) > 5 {
		d.canonname = values[5]
	}
	r.Type, r.Data, r.Class = object.ValueObject, d, R.Classes["Addrinfo"]
	return r
}
func addrinfoToSockaddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(packAddrinfoSockaddr(addrinfoDataOf(r)))
}
func addrinfoInspectSockaddr(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	if d.family == 1 {
		if strings.HasPrefix(d.unixPath, "/") {
			return rubyString(d.unixPath)
		}
		return rubyString("UNIX " + d.unixPath)
	}
	if d.port == 0 {
		return rubyString(d.ip)
	}
	if d.family == 10 {
		return rubyString(fmt.Sprintf("[%s]:%d", d.ip, d.port))
	}
	return rubyString(fmt.Sprintf("%s:%d", d.ip, d.port))
}
func addrinfoInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := addrinfoDataOf(r)
	where := stringRawValue(addrinfoInspectSockaddr(r))
	suffix := ""
	if d.family == 1 {
		if d.socktype == 2 {
			suffix = " SOCK_DGRAM"
		} else {
			suffix = " SOCK_STREAM"
		}
	} else if d.socktype == 1 {
		suffix = " TCP"
	} else if d.socktype == 2 {
		suffix = " UDP"
	}
	return rubyString("#<Addrinfo: " + where + suffix + ">")
}
