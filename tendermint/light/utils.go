package light

import (
	"fmt"
	"time"

	"github.com/celer-network/bsc-relayer/common"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/secp256k1"
	tmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/version"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func (pshp *PartSetHeader) FromType(psh *tmtypes.PartSetHeader) *PartSetHeader {
	if psh == nil {
		return pshp
	}
	pshp.Total = uint32(psh.Total)
	pshp.Hash = psh.Hash
	return pshp
}

func (bip *BlockID) FromType(bi *tmtypes.BlockID) *BlockID {
	if bi == nil {
		return bip
	}
	bip.Hash = bi.Hash
	bip.PartSetHeader = new(PartSetHeader).FromType(&bi.PartsHeader)
	return bip
}

func (tp *Timestamp) FromType(t time.Time) *Timestamp {
	tp.Seconds = t.Unix()
	tp.Nanos = int32(t.Nanosecond())
	return tp
}

func (csp *CommitSig) FromType(cs *tmtypes.CommitSig) *CommitSig {
	if cs == nil {
		return csp
	}
	csp.BlockIdFlag = BlockIDFlag_BLOCK_ID_FLAG_COMMIT
	csp.ValidatorAddress = cs.ValidatorAddress
	csp.Timestamp = new(Timestamp).FromType(cs.Timestamp)
	csp.Signature = cs.Signature
	return csp
}

func (cp *Commit) FromType(c *tmtypes.Commit) *Commit {
	if c == nil {
		return cp
	}
	cp.Height = c.Height()
	cp.Round = int32(c.Round())
	cp.BlockId = new(BlockID).FromType(&c.BlockID)
	sigs := make([]*CommitSig, len(c.Precommits))
	for i, sig := range c.Precommits {
		sigs[i] = new(CommitSig).FromType(sig)
	}
	cp.Signatures = sigs
	return cp
}

func (csp *Consensus) FromType(cs *version.Consensus) *Consensus {
	if cs == nil {
		return csp
	}
	csp.Block = cs.Block.Uint64()
	csp.App = cs.App.Uint64()
	return csp
}

func (lhp *LightHeader) FromType(h *tmtypes.Header) *LightHeader {
	if h == nil {
		return lhp
	}
	lhp.Version = new(Consensus).FromType(&h.Version)
	lhp.ChainId = h.ChainID
	lhp.Height = h.Height
	lhp.Time = new(Timestamp).FromType(h.Time)
	lhp.LastBlockId = new(BlockID).FromType(&h.LastBlockID)
	lhp.LastCommitHash = h.LastCommitHash
	lhp.DataHash = h.DataHash
	lhp.ValidatorsHash = h.ValidatorsHash
	lhp.NextValidatorsHash = h.NextValidatorsHash
	lhp.ConsensusHash = h.ConsensusHash
	lhp.AppHash = h.AppHash
	lhp.LastResultsHash = h.LastResultsHash
	lhp.EvidenceHash = h.EvidenceHash
	lhp.ProposerAddress = h.ProposerAddress
	return lhp
}

func (shp *SignedHeader) FromType(sh *tmtypes.SignedHeader) *SignedHeader {
	if sh == nil {
		return shp
	}
	shp.Header = new(LightHeader).FromType(sh.Header)
	shp.Commit = new(Commit).FromType(sh.Commit)
	return shp
}

func (pkp *PublicKey) FromType(pk crypto.PubKey) (*PublicKey, error) {
	switch k := pk.(type) {
	case ed25519.PubKeyEd25519:
		pkp.Sum = &PublicKey_Ed25519{
			Ed25519: k[:],
		}
	case secp256k1.PubKeySecp256k1:
		pkp.Sum = &PublicKey_Secp256K1{
			Secp256K1: k[:],
		}
	default:
		return pkp, fmt.Errorf("fromtype: key type %v is not supported", k)
	}
	return pkp, nil
}

func (vp *Validator) FromType(v *tmtypes.Validator) (*Validator, error) {
	if v == nil {
		return vp, fmt.Errorf("nil validator")
	}
	vp.Address = v.Address
	pk, err := new(PublicKey).FromType(v.PubKey)
	if err != nil {
		return vp, err
	}
	vp.PubKey = pk
	vp.VotingPower = v.VotingPower
	vp.ProposerPriority = v.ProposerPriority
	return vp, nil
}

func (vsp *ValidatorSet) FromType(vs *tmtypes.ValidatorSet) (*ValidatorSet, error) {
	if vs == nil {
		return vsp, fmt.Errorf("nil validator set")
	}
	vals := make([]*Validator, len(vs.Validators))
	for i, val := range vs.Validators {
		vp, err := new(Validator).FromType(val)
		if err != nil {
			return vsp, err
		}
		vals[i] = vp
	}
	p, err := new(Validator).FromType(vs.Proposer)
	if err != nil {
		return vsp, err
	}
	vsp.Validators = vals
	vsp.Proposer = p
	vsp.TotalVotingPower = vs.TotalVotingPower()
	return vsp, nil
}

func (tmh *TmHeader) FromType(h *common.Header) (*TmHeader, error) {
	if h == nil {
		return tmh, fmt.Errorf("nil header")
	}
	valSet, err := new(ValidatorSet).FromType(h.ValidatorSet)
	if err != nil {
		return tmh, err
	}
	tmh.SignedHeader = new(SignedHeader).FromType(&h.SignedHeader)
	tmh.ValidatorSet = valSet
	return tmh, nil
}

func EncodeHeader(h *common.Header) ([]byte, error) {
	tmh, err := new(TmHeader).FromType(h)
	if err != nil {
		return nil, err
	}
	bs, err := proto.Marshal(tmh)
	if err != nil {
		return nil, err
	}
	a := &anypb.Any{
		TypeUrl: "/tendermint.types.TmHeader",
		Value:   bs,
	}
	abs, err := proto.Marshal(a)
	if err != nil {
		return nil, err
	}
	return abs, nil
}
