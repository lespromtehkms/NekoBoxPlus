package yggdrasil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	golog "github.com/gologme/log"
	"github.com/sagernet/gvisor/pkg/buffer"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/icmp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/common/dialer"
	"github.com/sagernet/sing-box/common/iponly"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/bufio"
	"github.com/sagernet/sing/common/control"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"

	yggcore "github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/ipv6rwc"
)

var (
	_ adapter.OutboundWithPreferredRoutes = (*Endpoint)(nil)
	_ adapter.InterfaceUpdateListener     = (*Endpoint)(nil)
	_ adapter.OutboundWithMultiplex       = (*Endpoint)(nil)
	_ dialer.PacketDialerWithDestination  = (*Endpoint)(nil)
)

func RegisterEndpoint(registry *endpoint.Registry) {
	endpoint.Register[option.YggdrasilEndpointOptions](registry, C.TypeYggdrasil, NewEndpoint)
}

type Endpoint struct {
	endpoint.Adapter
	ctx               context.Context
	router            adapter.Router
	logger            logger.ContextLogger
	dnsRouter         adapter.DNSRouter
	network           adapter.NetworkManager
	platformInterface adapter.PlatformInterface
	options           option.YggdrasilEndpointOptions
	outboundDialer    N.Dialer

	core       *yggcore.Core
	rwc        *ipv6rwc.ReadWriteCloser
	stack      *stack.Stack
	linkEP     *channel.Endpoint
	localAddr6 netip.Addr
	started    atomic.Bool
}

func NewEndpoint(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.YggdrasilEndpointOptions) (adapter.Endpoint, error) {
	outboundDialer, err := dialer.NewWithOptions(dialer.Options{
		Context:          ctx,
		Options:          options.DialerOptions,
		RemoteIsDomain:   true,
		ResolverOnDetour: true,
		NewDialer:        true,
	})
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		Adapter:           endpoint.NewAdapterWithDialerOptions(C.TypeYggdrasil, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.DialerOptions),
		ctx:               ctx,
		router:            router,
		logger:            logger,
		dnsRouter:         service.FromContext[adapter.DNSRouter](ctx),
		network:           service.FromContext[adapter.NetworkManager](ctx),
		platformInterface: service.FromContext[adapter.PlatformInterface](ctx),
		options:           options,
		outboundDialer:    outboundDialer,
	}, nil
}

func (e *Endpoint) Start(stage adapter.StartStage, scope *adapter.Scope) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	return e.start(scope)
}

func (e *Endpoint) socketControlFunc() func(network, address string, c syscall.RawConn) error {
	if e.network != nil && e.network.AutoRedirectOutputMark() != 0 {
		return e.network.AutoRedirectOutputMarkFunc()
	}
	if e.platformInterface != nil && e.platformInterface.UsePlatformNetworkInterfaces() {
		if e.platformInterface.UsePlatformAutoDetectInterfaceControl() {
			return func(network, address string, conn syscall.RawConn) error {
				return control.Raw(conn, func(fileDescriptor uintptr) error {
					return e.platformInterface.AutoDetectInterfaceControl(int(fileDescriptor))
				})
			}
		}
		return nil
	}
	if e.network != nil {
		return e.network.AutoDetectInterfaceFunc()
	}
	return nil
}

func generateSelfSignedCert(priv ed25519.PrivateKey) (*tls.Certificate, error) {
	pub := priv.Public().(ed25519.PublicKey)
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * 10 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, pub, priv)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, nil
}

func parsePrivateKey(hexStr string) (ed25519.PrivateKey, error) {
	clean := strings.TrimSpace(hexStr)
	if clean == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	}
	b, err := hex.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid Yggdrasil private key hex: %w", err)
	}
	switch len(b) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	default:
		return nil, fmt.Errorf("invalid Yggdrasil private key length: %d bytes", len(b))
	}
}

func (e *Endpoint) start(scope *adapter.Scope) error {
	priv, err := parsePrivateKey(e.options.PrivateKey)
	if err != nil {
		return err
	}
	cert, err := generateSelfSignedCert(priv)
	if err != nil {
		return err
	}

	var opts []yggcore.SetupOption
	opts = append(opts, yggcore.NodeInfoPrivacy(true))
	if e.options.GroupPassword != "" {
		opts = append(opts, yggcore.GroupPassword(e.options.GroupPassword))
	}
	for _, rawKey := range e.options.AllowedPublicKeys {
		for _, token := range strings.FieldsFunc(rawKey, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
		}) {
			token = strings.TrimSpace(token)
			if token == "" || strings.HasPrefix(token, "#") {
				continue
			}
			kb, decErr := hex.DecodeString(token)
			if decErr != nil || len(kb) != ed25519.PublicKeySize {
				e.logger.Warn("ignoring invalid Yggdrasil AllowedPublicKey: ", token)
				continue
			}
			opts = append(opts, yggcore.AllowedPublicKey(ed25519.PublicKey(kb)))
		}
	}

	yggLogger := golog.New(io.Discard, "", 0)
	ycore, err := yggcore.New(cert, yggLogger, opts...)
	if err != nil {
		return E.Cause(err, "create Yggdrasil core")
	}

	if ctrl := e.socketControlFunc(); ctrl != nil {
		ycore.SetSocketControl(ctrl)
	}
	if e.options.Detour != "" && e.outboundDialer != nil {
		ycore.SetCustomDialContext(func(ctx context.Context, network, address string) (net.Conn, error) {
			return e.outboundDialer.DialContext(ctx, network, M.ParseSocksaddr(address))
		})
	}

	for _, rawPeer := range e.options.Peers {
		for _, peerURI := range strings.FieldsFunc(rawPeer, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
		}) {
			peerURI = strings.TrimSpace(peerURI)
			if peerURI == "" || strings.HasPrefix(peerURI, "#") {
				continue
			}
			u, parseErr := url.Parse(peerURI)
			if parseErr != nil {
				e.logger.Warn("invalid Yggdrasil peer URI ", peerURI, ": ", parseErr)
				continue
			}
			if addErr := ycore.AddPeer(u, ""); addErr != nil {
				e.logger.Warn("failed to add Yggdrasil peer ", peerURI, ": ", addErr)
			}
		}
	}

	rwc := ipv6rwc.NewReadWriteCloser(ycore)
	ip6 := ycore.Address().To16()
	if ip6 == nil {
		_ = rwc.Close()
		ycore.Stop()
		return E.New("failed to get Yggdrasil IPv6 address")
	}
	var addr16 [16]byte
	copy(addr16[:], ip6)
	e.localAddr6 = netip.AddrFrom16(addr16)

	mtu := e.options.MTU
	if mtu == 0 {
		mtu = uint32(rwc.MaxMTU())
		if mtu == 0 || mtu > 65535 {
			mtu = 65535
		}
	}
	rwc.SetMTU(uint64(mtu))

	linkEP := channel.New(512, mtu, "")
	ipStack := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol6},
	})
	if gErr := ipStack.CreateNIC(1, linkEP); gErr != nil {
		_ = rwc.Close()
		ycore.Stop()
		return gonet.TranslateNetstackError(gErr)
	}
	protoAddr := tcpip.ProtocolAddress{
		Protocol: header.IPv6ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom16(addr16),
			PrefixLen: 7,
		},
	}
	if gErr := ipStack.AddProtocolAddress(1, protoAddr, stack.AddressProperties{}); gErr != nil {
		ipStack.Close()
		_ = rwc.Close()
		ycore.Stop()
		return gonet.TranslateNetstackError(gErr)
	}
	_ = ipStack.SetSpoofing(1, true)
	_ = ipStack.SetPromiscuousMode(1, true)
	ipStack.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv6EmptySubnet,
			NIC:         1,
		},
	})

	runCtx, cancel := context.WithCancel(scope.Context())

	// Outbound packets: gVisor -> Yggdrasil ipv6rwc
	go func() {
		for {
			pkt := linkEP.ReadContext(runCtx)
			if pkt == nil {
				return
			}
			view := pkt.ToView()
			_, _ = rwc.Write(view.AsSlice())
			view.Release()
			pkt.DecRef()
		}
	}()

	// Inbound packets: Yggdrasil ipv6rwc -> gVisor
	go func() {
		buf := make([]byte, 65535)
		for {
			n, readErr := rwc.Read(buf)
			if readErr != nil || runCtx.Err() != nil {
				return
			}
			if n == 0 {
				continue
			}
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(append([]byte(nil), buf[:n]...)),
			})
			linkEP.InjectInbound(header.IPv6ProtocolNumber, pkt)
			pkt.DecRef()
		}
	}()

	e.core = ycore
	e.rwc = rwc
	e.stack = ipStack
	e.linkEP = linkEP
	e.started.Store(true)
	e.logger.Info("Yggdrasil endpoint started, address: ", e.localAddr6.String())

	scope.Add(func() error {
		e.started.Store(false)
		cancel()
		linkEP.Close()
		ipStack.Close()
		_ = rwc.Close()
		ycore.Stop()
		return nil
	})
	return nil
}

func (e *Endpoint) MultiplexEnabled() bool {
	return true
}

func (e *Endpoint) InterfaceUpdated(ctx context.Context) {
	if !e.started.Load() || e.core == nil {
		return
	}
	e.core.RetryPeersNow()
}

func (e *Endpoint) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	if !e.started.Load() {
		return nil, E.New("Yggdrasil is not ready yet")
	}
	if destination.IsDomain() && e.dnsRouter != nil {
		destinationAddresses, err := e.dnsRouter.Lookup(ctx, destination.Fqdn, adapter.DNSQueryOptions{})
		if err != nil {
			return nil, err
		}
		return N.DialSerial(ctx, e, network, destination, destinationAddresses)
	}
	if !destination.IsIPv6() {
		return nil, E.New("Yggdrasil only supports IPv6 destinations, got: ", destination)
	}
	localAddr := tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFrom16(e.localAddr6.As16()),
	}
	remoteAddr := tcpip.FullAddress{
		NIC:  1,
		Port: destination.Port,
		Addr: tcpip.AddrFrom16(destination.Addr.As16()),
	}
	switch N.NetworkName(network) {
	case N.NetworkTCP:
		return gonet.DialTCPWithBind(ctx, e.stack, localAddr, remoteAddr, header.IPv6ProtocolNumber)
	case N.NetworkUDP:
		return gonet.DialUDP(e.stack, &localAddr, &remoteAddr, header.IPv6ProtocolNumber)
	default:
		return nil, E.Extend(N.ErrUnknownNetwork, network)
	}
}

func (e *Endpoint) listenPacketWithAddress(destination M.Socksaddr) (net.PacketConn, error) {
	if !e.started.Load() {
		return nil, E.New("Yggdrasil is not ready yet")
	}
	if destination.IsValid() && destination.IsIPv4() {
		return nil, E.New("Yggdrasil only supports IPv6 UDP packets")
	}
	bind := tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFrom16(e.localAddr6.As16()),
	}
	return gonet.DialUDP(e.stack, &bind, nil, header.IPv6ProtocolNumber)
}

func (e *Endpoint) ListenPacketWithDestination(ctx context.Context, destination M.Socksaddr) (net.PacketConn, netip.Addr, error) {
	if destination.IsDomain() && e.dnsRouter != nil {
		destinationAddresses, err := e.dnsRouter.Lookup(ctx, destination.Fqdn, adapter.DNSQueryOptions{})
		if err != nil {
			return nil, netip.Addr{}, err
		}
		var errors []error
		for _, address := range destinationAddresses {
			packetConn, packetErr := e.listenPacketWithAddress(M.SocksaddrFrom(address, destination.Port))
			if packetErr == nil {
				return iponly.NewPacketConn(e.logger, packetConn), address, nil
			}
			errors = append(errors, packetErr)
		}
		return nil, netip.Addr{}, E.Errors(errors...)
	}
	packetConn, err := e.listenPacketWithAddress(destination)
	if err != nil {
		return nil, netip.Addr{}, err
	}
	if destination.IsIP() {
		return iponly.NewPacketConn(e.logger, packetConn), destination.Addr, nil
	}
	return iponly.NewPacketConn(e.logger, packetConn), netip.Addr{}, nil
}

func (e *Endpoint) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	packetConn, destinationAddress, err := e.ListenPacketWithDestination(ctx, destination)
	if err != nil {
		return nil, err
	}
	if destinationAddress.IsValid() && destination != M.SocksaddrFrom(destinationAddress, destination.Port) {
		return bufio.NewNATPacketConn(bufio.NewPacketConn(packetConn), M.SocksaddrFrom(destinationAddress, destination.Port), destination), nil
	}
	return packetConn, nil
}

func (e *Endpoint) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Inbound = e.Tag()
	metadata.InboundType = e.Type()
	metadata.Source = source
	metadata.Destination = destination
	e.router.RouteConnectionEx(ctx, conn, metadata, onClose)
}

func (e *Endpoint) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	var metadata adapter.InboundContext
	metadata.Inbound = e.Tag()
	metadata.InboundType = e.Type()
	metadata.Source = source
	metadata.Destination = destination
	e.router.RoutePacketConnectionEx(ctx, conn, metadata, onClose)
}

func (e *Endpoint) PreferredDomain(metadata *adapter.InboundContext, domain string) bool {
	return false
}

func (e *Endpoint) PreferredAddress(metadata *adapter.InboundContext, address netip.Addr) bool {
	if !e.started.Load() || !address.Is6() {
		return false
	}
	b := address.As16()
	return (b[0] & 0xfe) == 0x02
}
