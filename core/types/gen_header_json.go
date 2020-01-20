// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/clearmatics/autonity/common"
	"github.com/clearmatics/autonity/common/hexutil"
)

var _ = (*headerMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
		UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
		Coinbase    common.Address `json:"miner"            gencodec:"required"`
		Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
		TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
		Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
		Difficulty  *hexutil.Big   `json:"difficulty"       gencodec:"required"`
		Number      *hexutil.Big   `json:"number"           gencodec:"required"`
		GasLimit    hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
		GasUsed     hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
		Time        hexutil.Uint64 `json:"timestamp"        gencodec:"required"`
		Extra       hexutil.Bytes  `json:"extraData"        gencodec:"required"`
		MixDigest   common.Hash    `json:"mixHash"`
		Nonce       BlockNonce     `json:"nonce"`
		Hash        common.Hash    `json:"hash"`
	}
	type ExtraHeader struct {
		Committee          Committee       `json:"committee"           gencodec:"required"`
		ProposerSeal       hexutil.Bytes   `json:"proposerSeal"        gencodec:"required"`
		Round              *hexutil.Big    `json:"round"               gencodec:"required"`
		CommittedSeals     []hexutil.Bytes `json:"committedSeals"      gencodec:"required"`
		PastCommittedSeals []hexutil.Bytes `json:"pastCommittedSeals"  gencodec:"required"`
	}

	var enc Header
	var encExtra ExtraHeader

	enc.ParentHash = h.ParentHash
	enc.UncleHash = h.UncleHash
	enc.Coinbase = h.Coinbase
	enc.Root = h.Root
	enc.TxHash = h.TxHash
	enc.ReceiptHash = h.ReceiptHash
	enc.Bloom = h.Bloom
	enc.Difficulty = (*hexutil.Big)(h.Difficulty)
	enc.Number = (*hexutil.Big)(h.Number)
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Time = hexutil.Uint64(h.Time)
	enc.MixDigest = h.MixDigest
	enc.Nonce = h.Nonce

	encExtra.Committee = h.Committee
	encExtra.ProposerSeal = h.ProposerSeal
	encExtra.Round = (*hexutil.Big)(h.Round)
	if encExtra.CommittedSeals != nil {
		encExtra.CommittedSeals = make([]hexutil.Bytes, len(h.CommittedSeals))
		for k, v := range h.CommittedSeals {
			encExtra.CommittedSeals[k] = v
		}
	}
	if h.PastCommittedSeals != nil {
		encExtra.PastCommittedSeals = make([]hexutil.Bytes, len(h.PastCommittedSeals))
		for k, v := range h.PastCommittedSeals {
			encExtra.PastCommittedSeals[k] = v
		}
	}

	extraBytes, err := json.Marshal(&encExtra)
	if err != nil {
		return nil, err
	}
	//fixme: надо ли делать проверки на пустоту подписей и прочего?
	enc.Extra = extraBytes

	enc.Hash = h.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		ParentHash  *common.Hash    `json:"parentHash"       gencodec:"required"`
		UncleHash   *common.Hash    `json:"sha3Uncles"       gencodec:"required"`
		Coinbase    *common.Address `json:"miner"            gencodec:"required"`
		Root        *common.Hash    `json:"stateRoot"        gencodec:"required"`
		TxHash      *common.Hash    `json:"transactionsRoot" gencodec:"required"`
		ReceiptHash *common.Hash    `json:"receiptsRoot"     gencodec:"required"`
		Bloom       *Bloom          `json:"logsBloom"        gencodec:"required"`
		Difficulty  *hexutil.Big    `json:"difficulty"       gencodec:"required"`
		Number      *hexutil.Big    `json:"number"           gencodec:"required"`
		GasLimit    *hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
		GasUsed     *hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
		Time        *hexutil.Uint64 `json:"timestamp"        gencodec:"required"`
		Extra       *hexutil.Bytes  `json:"extraData"        gencodec:"required"`
		MixDigest   *common.Hash    `json:"mixHash"`
		Nonce       *BlockNonce     `json:"nonce"`
	}
	type ExtraHeader struct {
		Committee          *Committee       `json:"committee"           gencodec:"required"`
		ProposerSeal       *hexutil.Bytes   `json:"proposerSeal"        gencodec:"required"`
		Round              *hexutil.Big     `json:"round"               gencodec:"required"`
		CommittedSeals     *[]hexutil.Bytes `json:"committedSeals"      gencodec:"required"`
		PastCommittedSeals *[]hexutil.Bytes `json:"pastCommittedSeals"  gencodec:"required"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Extra == nil {
		return errors.New("missing required field 'extraData' for Header")
	}

	var decExtra ExtraHeader
	if err := json.Unmarshal(*dec.Extra, &decExtra); err != nil {
		return err
	}

	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.UncleHash == nil {
		return errors.New("missing required field 'sha3Uncles' for Header")
	}
	h.UncleHash = *dec.UncleHash
	if dec.Coinbase == nil {
		return errors.New("missing required field 'miner' for Header")
	}
	h.Coinbase = *dec.Coinbase
	if dec.Root == nil {
		return errors.New("missing required field 'stateRoot' for Header")
	}
	h.Root = *dec.Root
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionsRoot' for Header")
	}
	h.TxHash = *dec.TxHash
	if dec.ReceiptHash == nil {
		return errors.New("missing required field 'receiptsRoot' for Header")
	}
	h.ReceiptHash = *dec.ReceiptHash
	if dec.Bloom == nil {
		return errors.New("missing required field 'logsBloom' for Header")
	}
	h.Bloom = *dec.Bloom
	if dec.Difficulty == nil {
		return errors.New("missing required field 'difficulty' for Header")
	}
	h.Difficulty = (*big.Int)(dec.Difficulty)
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = (*big.Int)(dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Time == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Time = uint64(*dec.Time)
	if dec.MixDigest != nil {
		h.MixDigest = *dec.MixDigest
	}
	if dec.Nonce != nil {
		h.Nonce = *dec.Nonce
	}

	//fixme надо ли делать обнуление?
	h.Extra = nil

	if decExtra.Committee == nil {
		return errors.New("missing required field 'committee' for Header")
	}
	h.Committee = *decExtra.Committee
	if decExtra.ProposerSeal == nil {
		return errors.New("missing required field 'proposerSeal' for Header")
	}
	h.ProposerSeal = *decExtra.ProposerSeal
	if decExtra.Round == nil {
		return errors.New("missing required field 'round' for Header")
	}
	h.Round = (*big.Int)(decExtra.Round)
	if decExtra.CommittedSeals == nil {
		return errors.New("missing required field 'committedSeals' for Header")
	}
	h.CommittedSeals = make([][]byte, len(*decExtra.CommittedSeals))
	for k, v := range *decExtra.CommittedSeals {
		h.CommittedSeals[k] = v
	}
	if decExtra.PastCommittedSeals == nil {
		return errors.New("missing required field 'pastCommittedSeals' for Header")
	}
	h.PastCommittedSeals = make([][]byte, len(*decExtra.PastCommittedSeals))
	for k, v := range *decExtra.PastCommittedSeals {
		h.PastCommittedSeals[k] = v
	}
	return nil
}
