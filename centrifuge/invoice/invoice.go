package invoice

import (
	"crypto/sha256"
	"github.com/CentrifugeInc/centrifuge-protobufs/documenttypes"
	"github.com/CentrifugeInc/centrifuge-protobufs/gen/go/coredocument"
	"github.com/CentrifugeInc/centrifuge-protobufs/gen/go/invoice"
	"github.com/CentrifugeInc/go-centrifuge/centrifuge/coredocument"
	"github.com/centrifuge/precise-proofs/proofs"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("invoice")

type Invoice struct {
	Document *invoicepb.InvoiceDocument
}

func NewInvoice(invDoc *invoicepb.InvoiceDocument) *Invoice {
	inv := &Invoice{invDoc}
	// IF salts have not been provided, let's generate them
	if invDoc.Salts == nil {
		invoiceSalts := invoicepb.InvoiceDataSalts{}
		proofs.FillSalts(&invoiceSalts)
		inv.Document.Salts = &invoiceSalts
	}
	return inv
}

func NewEmptyInvoice() *Invoice {
	invoiceSalts := invoicepb.InvoiceDataSalts{}
	proofs.FillSalts(&invoiceSalts)
	doc := invoicepb.InvoiceDocument{
		CoreDocument: &coredocumentpb.CoreDocument{},
		Data:         &invoicepb.InvoiceData{},
		Salts:        &invoiceSalts,
	}
	return &Invoice{&doc}
}

func NewInvoiceFromCoreDocument(coredocument *coredocument.CoreDocument) (inv *Invoice) {
	if coredocument.Document.EmbeddedData.TypeUrl != documenttypes.InvoiceDataTypeUrl ||
		coredocument.Document.EmbeddedDataSalts.TypeUrl != documenttypes.InvoiceSaltsTypeUrl {
		log.Fatal("Trying to convert document with incorrect schema")
	}

	invoiceData := &invoicepb.InvoiceData{}
	proto.Unmarshal(coredocument.Document.EmbeddedData.Value, invoiceData)

	invoiceSalts := &invoicepb.InvoiceDataSalts{}
	proto.Unmarshal(coredocument.Document.EmbeddedDataSalts.Value, invoiceSalts)

	emptiedCoreDoc := coredocumentpb.CoreDocument{}
	proto.Merge(&emptiedCoreDoc, coredocument.Document)
	emptiedCoreDoc.EmbeddedData = nil
	emptiedCoreDoc.EmbeddedDataSalts = nil
	inv = NewEmptyInvoice()
	inv.Document.Data = invoiceData
	inv.Document.Salts = invoiceSalts
	inv.Document.CoreDocument = &emptiedCoreDoc
	return
}

func (inv *Invoice) getDocumentTree() (tree *proofs.DocumentTree, err error) {
	t := proofs.NewDocumentTree()
	sha256Hash := sha256.New()
	t.SetHashFunc(sha256Hash)
	err = t.FillTree(inv.Document.Data, inv.Document.Salts)
	if err != nil {
		log.Error("getDocumentTree:", err)
		return nil, err
	}
	return &t, nil
}

func (inv *Invoice) CalculateMerkleRoot() error {
	tree, err := inv.getDocumentTree()
	if err != nil {
		return err
	}
	// TODO: below should actually be stored as CoreDocument.DataMerkleRoot
	inv.Document.CoreDocument.DocumentRoot = tree.RootHash()
	return nil
}

func (inv *Invoice) CreateProofs(fields []string) (proofs []*proofs.Proof, err error) {
	tree, err := inv.getDocumentTree()
	if err != nil {
		log.Error(err)
		return nil, err
	}
	for _, field := range fields {
		proof, err := tree.CreateProof(field)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		proofs = append(proofs, &proof)
	}
	return
}

func (inv *Invoice) ConvertToCoreDocument() (coredocument coredocument.CoreDocument) {
	coredocpb := &coredocumentpb.CoreDocument{}
	proto.Merge(coredocpb, inv.Document.CoreDocument)
	serializedInvoice, err := proto.Marshal(inv.Document.Data)
	if err != nil {
		log.Fatalf("Could not serialize InvoiceData: %s", err)
	}

	invoiceAny := any.Any{
		TypeUrl: documenttypes.InvoiceDataTypeUrl,
		Value:   serializedInvoice,
	}

	serializedSalts, err := proto.Marshal(inv.Document.Salts)
	if err != nil {
		log.Fatalf("Could not serialize InvoiceSalts: %s", err)
	}

	invoiceSaltsAny := any.Any{
		TypeUrl: documenttypes.InvoiceSaltsTypeUrl,
		Value:   serializedSalts,
	}

	coredocpb.EmbeddedData = &invoiceAny
	coredocpb.EmbeddedDataSalts = &invoiceSaltsAny
	coredocument.Document = coredocpb
	return
}