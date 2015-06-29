package blockexplorer

import (
	"bytes"
	"errors"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

const (
	responseBlock        = "Block"
	responseTransaction  = "Transaction"
	responseFileContract = "FileContract"
	responseOutput       = "Output"
	responseAddress      = "Address"
)

type (
	// Many explicit wrappers for return values
	// All must have the ResponseType field as a string
	blockResponse struct {
		Block        types.Block
		Height       types.BlockHeight
		ResponseType string
	}

	// Wrapper for a transaction, with a little extra info
	transactionResponse struct {
		Tx           types.Transaction
		ParentID     types.BlockID
		TxNum        int
		ResponseType string
	}

	// Wrapper for fcInfo struct, defined in database.go
	fcResponse struct {
		Contract     crypto.Hash
		Revisions    []crypto.Hash
		Proof        crypto.Hash
		ResponseType string
	}

	// Wrapper for the address type response
	addrResponse struct {
		Txns         []crypto.Hash
		ResponseType string
	}

	outputResponse struct {
		OutputTx     crypto.Hash
		InputTx      crypto.Hash
		ResponseType string
	}
)

// GetHashInfo returns sufficient data about the hash that was
// provided to do more extensive lookups
func (be *BlockExplorer) GetHashInfo(hash []byte) (interface{}, error) {
	if len(hash) < crypto.HashSize {
		return nil, errors.New("requested hash not long enough")
	}

	lockID := be.mu.RLock()
	defer be.mu.RUnlock(lockID)

	// Perform a lookup to tell which type of hash it is
	typeBytes, err := be.db.GetFromBucket("Hashes", hash[:crypto.HashSize])
	if err != nil {
		return nil, err
	}
	if typeBytes == nil {
		return nil, errors.New("requested hash not found in database")
	}

	var hashType int
	err = encoding.Unmarshal(typeBytes, &hashType)

	switch hashType {
	case hashBlock:
		var id types.BlockID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getBlock(types.BlockID(id))
	case hashTransaction:
		var id crypto.Hash
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getTransaction(id)
	case hashFilecontract:
		var id types.FileContractID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getFileContract(id)
	case hashCoinOutputID:
		var id types.SiacoinOutputID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getSiacoinOutput(id)
	case hashFundOutputID:
		var id types.SiafundOutputID
		copy(id[:], hash[:crypto.HashSize])
		return be.db.getSiafundOutput(id)
	case hashUnlockHash:
		var id types.UnlockHash
		copy(id[:], hash[:crypto.HashSize])

		// Check that the address is valid before doing a lookup
		if len(hash) != crypto.HashSize+types.UnlockHashChecksumSize {
			return nil, errors.New("address does not have a valid checksum")
		}
		givenChecksum := hash[crypto.HashSize : crypto.HashSize+types.UnlockHashChecksumSize]
		uhChecksum := crypto.HashObject(id)
		if bytes.Compare(givenChecksum, uhChecksum[:types.UnlockHashChecksumSize]) != 0 {
			return nil, errors.New("address does not have a valid checksum")
		}

		return be.db.getAddressTransactions(id)
	default:
		return nil, errors.New("bad hash type")
	}
}

// Returns the block with a given id
func (db *explorerDB) getBlock(id types.BlockID) (block blockResponse, err error) {
	b, err := db.GetFromBucket("Blocks", encoding.Marshal(id))
	if err != nil {
		return block, err
	}

	var bd blockData
	err = encoding.Unmarshal(b, &bd)
	if err != nil {
		return block, err
	}
	block.Block = bd.Block
	block.Height = bd.Height
	block.ResponseType = responseBlock
	return block, nil
}

// Returns the transaction with the given id
func (db *explorerDB) getTransaction(id crypto.Hash) (transactionResponse, error) {
	var tr transactionResponse

	// Look up the transaction's location
	tBytes, err := db.GetFromBucket("Transactions", encoding.Marshal(id))
	if err != nil {
		return tr, err
	}

	var tLocation txInfo
	err = encoding.Unmarshal(tBytes, &tLocation)
	if err != nil {
		return tr, err
	}

	// Look up the block specified by the location and extract the transaction
	bBytes, err := db.GetFromBucket("Blocks", encoding.Marshal(tLocation.BlockID))
	if err != nil {
		return tr, err
	}

	var block types.Block
	err = encoding.Unmarshal(bBytes, &block)
	if err != nil {
		return tr, err
	}
	tr.Tx = block.Transactions[tLocation.TxNum]
	tr.ParentID = tLocation.BlockID
	tr.TxNum = tLocation.TxNum
	tr.ResponseType = responseTransaction
	return tr, nil
}

// Returns the list of transactions a file contract with a given id has taken part in
func (db *explorerDB) getFileContract(id types.FileContractID) (fcResponse, error) {
	var fr fcResponse
	fcBytes, err := db.GetFromBucket("FileContracts", encoding.Marshal(id))
	if err != nil {
		return fr, err
	}

	var fc fcInfo
	err = encoding.Unmarshal(fcBytes, &fc)
	if err != nil {
		return fr, err
	}

	fr.Contract = fc.Contract
	fr.Revisions = fc.Revisions
	fr.Proof = fc.Proof
	fr.ResponseType = responseFileContract

	return fr, nil
}

func (db *explorerDB) getSiacoinOutput(id types.SiacoinOutputID) (outputResponse, error) {
	var or outputResponse
	otBytes, err := db.GetFromBucket("SiacoinOutputs", encoding.Marshal(id))
	if err != nil {
		return or, err
	}

	var ot outputTransactions
	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return or, err
	}

	or.OutputTx = ot.OutputTx
	or.InputTx = ot.InputTx
	or.ResponseType = responseOutput

	return or, nil
}

func (db *explorerDB) getSiafundOutput(id types.SiafundOutputID) (outputResponse, error) {
	var or outputResponse
	otBytes, err := db.GetFromBucket("SiafundOutputs", encoding.Marshal(id))
	if err != nil {
		return or, err
	}

	var ot outputTransactions
	err = encoding.Unmarshal(otBytes, &ot)
	if err != nil {
		return or, err
	}

	or.OutputTx = ot.OutputTx
	or.InputTx = ot.InputTx
	or.ResponseType = responseOutput

	return or, nil
}

func (db *explorerDB) getAddressTransactions(address types.UnlockHash) (addrResponse, error) {
	var ar addrResponse
	txBytes, err := db.GetFromBucket("Addresses", encoding.Marshal(address))
	if err != nil {
		return ar, err
	}

	var atxids []crypto.Hash
	err = encoding.Unmarshal(txBytes, &atxids)
	if err != nil {
		return ar, err
	}

	ar.Txns = atxids
	ar.ResponseType = responseAddress

	return ar, nil
}
