package core

import (
	"fmt"
	"net"
	"time"

	"github.com/drand/drand/beacon"
	"github.com/drand/drand/key"
	"github.com/drand/drand/protobuf/crypto/dkg"
	pdkg "github.com/drand/drand/protobuf/crypto/dkg"
	"github.com/drand/drand/protobuf/drand"
	proto "github.com/drand/drand/protobuf/drand"
	"github.com/drand/kyber"
)

func ProtoToGroup(g *proto.GroupPacket) (*key.Group, error) {
	var nodes = make([]*key.Identity, 0, len(g.GetNodes()))
	for _, id := range g.GetNodes() {
		kid, err := protoToIdentity(id)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, kid)
	}
	n := len(nodes)
	thr := int(g.GetThreshold())
	if thr < key.MinimumT(n) {
		return nil, fmt.Errorf("invalid threshold: %d vs %d (minimum)", thr, key.MinimumT(n))
	}
	genesisTime := int64(g.GetGenesisTime())
	if genesisTime == 0 {
		return nil, fmt.Errorf("genesis time zero")
	}
	period := time.Duration(g.GetPeriod()) * time.Second
	if period == time.Duration(0) {
		return nil, fmt.Errorf("period time is zero")
	}
	var dist = new(key.DistPublic)
	for _, coeff := range g.DistKey {
		c := key.KeyGroup.Point()
		if err := c.UnmarshalBinary(coeff); err != nil {
			return nil, fmt.Errorf("invalid distributed key coefficients:%v", err)
		}
		dist.Coefficients = append(dist.Coefficients, c)
	}
	group := key.NewGroup(nodes, thr, genesisTime)
	// XXX Change the group creation methods to avoid this
	group.Period = period
	group.TransitionTime = int64(g.GetTransitionTime())
	if g.GetGenesisSeed() != nil {
		group.GenesisSeed = g.GetGenesisSeed()
	}
	if len(dist.Coefficients) > 0 {
		group.PublicKey = dist
	}
	return group, nil
}

func groupToProto(g *key.Group) *proto.GroupPacket {
	var out = new(proto.GroupPacket)
	var ids = make([]*proto.Identity, len(g.Nodes))
	for i, id := range g.Nodes {
		key, _ := id.Key.MarshalBinary()
		ids[i] = &proto.Identity{
			Address: id.Address(),
			Tls:     id.IsTLS(),
			Key:     key,
		}
	}
	out.Nodes = ids
	out.Period = uint32(g.Period.Seconds())
	out.Threshold = uint32(g.Threshold)
	out.GenesisTime = uint64(g.GenesisTime)
	out.TransitionTime = uint64(g.TransitionTime)
	out.GenesisSeed = g.GetGenesisSeed()
	if g.PublicKey != nil {
		var coeffs = make([][]byte, len(g.PublicKey.Coefficients))
		for i, c := range g.PublicKey.Coefficients {
			buff, _ := c.MarshalBinary()
			coeffs[i] = buff
		}
		out.DistKey = coeffs
	}
	return out
}

func protoToIdentity(n *proto.Identity) (*key.Identity, error) {
	_, _, err := net.SplitHostPort(n.GetAddress())
	if err != nil {
		return nil, err
	}
	public := key.KeyGroup.Point()
	if err := public.UnmarshalBinary(n.GetKey()); err != nil {
		return nil, err
	}
	return &key.Identity{
		Addr: n.GetAddress(),
		TLS:  n.Tls,
		Key:  public,
	}, nil
}

func beaconToProto(b *beacon.Beacon) *drand.PublicRandResponse {
	return &drand.PublicRandResponse{
		Round:             b.Round,
		Signature:         b.Signature,
		PreviousRound:     b.PreviousRound,
		PreviousSignature: b.PreviousSig,
		Randomness:        b.Randomness(),
	}
}

func protoToDeal(d *pdkg.DealBundle) (*dkg.DealBundle, error) {
	bundle := new(DealBundle)
	bundle.DealerIndex = d.DealerIndex
	publics := make([]kyber.Point, 0, len(p.Commits))
	for _, c := range d.Commits {
		coeff := key.KeyGroup.Point()
		if err := coeff.UnmarshalBinary(c); err != nil {
			return nil, fmt.Errorf("invalid public coeff:%s", err)
		}
		publics = append(publics, coeff)
	}
	bundle.Public = publics
	deals := make([]*dkg.Deal, 0, len(p.Deals))
	for _, d := range d.Deals {
		deal := &dkg.Deal{
			EncryptedShare: d.EncryptedShare,
			Index:          d.Index,
		}
		deals = append(deals, deal)
	}
	bundle.Deals = deals
	return bundle, nil
}

func protoToResp(r *pdkg.ResponseBundle) *dkg.ResponseBundle {
	resp = new(dkg.ResponseBundle)
	resp.ShareIndex = r.ShareIndex
	resp.Responses = make([]*dkg.Response, 0, len(r.Responses))
	for _, rr := range r.Responses {
		response := &dkg.Response{
			DealerIndex: rr.DealerIndex,
			Status:      rr.Status,
		}
		resp.Responses = append(resp.Responses, response)
	}
	return resp
}

func protoToJustif(j *pdkg.JustifBundle) (*dkg.JustificationBundle, error) {
	just := new(dkg.JustificationBundle)
	just.ShareIndex = j.ShareIndex
	just.Justifications = make([]*dkg.Justification, len(j.Justifications))
	for i, j := range j.Justifications {
		share := key.KeyGroup.Scalar()
		if err := share.UnmarshalBinary(j.Share); err != nil {
			return nil, fmt.Errorf("invalid share: %s", err)
		}
		just := &dkg.Justification{
			ShareIndex: j.ShareIndex,
			Share:      share,
		}
		just.Justifications[i] = just
	}
	return just, nil
}

func dealToProto(d *dkg.AuthDealBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.Packet_Deal)
	bundle.DealerIndex = d.Bundle.DealerIndex
	bundle.Deals = make([]*pdkg.Deal, len(d.Bundle.Deals))
	for i, deal := range d.Bundle.Deals {
		pdeal := &pdkg.Deal{
			ShareIndex:     deal.ShareIndex,
			EncryptedShare: deal.EncryptedShare,
		}
		bundle.Deals[i] = pdeal
	}
	packet.Signature = d.Signature
	packet.Bundle = bundle
	return packet
}

func respToProto(r *dkg.AuthResponseBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.Packet_Response)
	bundle.ShareIndex = d.Bundle.ShareIndex
	bundle.Responses = make([]*pdkg.Response, len(d.Bundle.Responses))
	for i, resp := range d.Bundle.Responses {
		presp := &pdkg.Response{
			DealerIndex: resp.DealerIndex,
			Status:      resp.Status,
		}
		bundle.Responses[i] = presp
	}
	packet.Signature = d.Signature
	packet.Bundle = bundle
	return packet
}

func justifToProto(j *dkg.AuthJustifBundle) *pdkg.Packet {
	packet := new(pdkg.Packet)
	bundle := new(pdkg.Packet_Justification)
	bundle.DealerIndex = d.Bundle.DealerIndex
	bundle.Justifications = make([]*pdkg.Justification, len(d.Bundle.Justifications))
	for i, just := range d.Bundle.Justifications {
		shareBuff, _ := just.Share.MarshalBinary()
		pjust := &pdkg.Justification{
			ShareIndex: just.ShareIndex,
			Share:      shareBuff,
		}
		bundle.Justfications[i] = pjust
	}
	packet.Signature = d.Signature
	packet.Bundle = bundle
	return packet
}
