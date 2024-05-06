package internal

import (
	"strings"

	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc/resolver/internal/targets"
	"google.golang.org/grpc/resolver"
)

type discovBuilder struct {
	cc     resolver.ClientConn
	update func()
}

func (b *discovBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (
	resolver.Resolver, error) {
	b.cc = cc
	if err := b.updateState(target); err != nil {
		return nil, err
	}

	return &nopResolver{cc: cc}, nil
}

func (b *discovBuilder) Scheme() string {
	return DiscovScheme
}

func (b *discovBuilder) updateState(target resolver.Target) error {
	if b.update == nil {
		if err := b.buildEndpointsUpdater(target); err != nil {
			return err
		}
	}

	b.update()

	return nil
}

func (b *discovBuilder) buildEndpointsUpdater(target resolver.Target) error {
	hosts := strings.FieldsFunc(targets.GetAuthority(target), func(r rune) bool {
		return r == EndpointSepChar
	})
	sub, err := discov.NewSubscriber(hosts, targets.GetEndpoints(target))
	if err != nil {
		return err
	}

	update := func() {
		var addrs []resolver.Address
		for _, val := range subset(sub.Values(), subsetSize) {
			addrs = append(addrs, resolver.Address{
				Addr: val,
			})
		}
		if err := b.cc.UpdateState(resolver.State{
			Addresses: addrs,
		}); err != nil {
			logx.Error(err)
		}
	}
	sub.AddListener(update)
	b.update = update

	return nil
}
