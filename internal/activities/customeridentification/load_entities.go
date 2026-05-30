package customeridentification

import (
	"context"
	"errors"

	"github.com/abhinavxd/libredesk/internal/contract"
	contractmodels "github.com/abhinavxd/libredesk/internal/contract/models"
	"github.com/abhinavxd/libredesk/internal/customer"
	customermodels "github.com/abhinavxd/libredesk/internal/customer/models"
	cimodels "github.com/abhinavxd/libredesk/internal/workflows/customeridentification"
	"golang.org/x/sync/errgroup"
)

func (a *Activities) LoadContractAndCustomerActivity(ctx context.Context, in cimodels.LoadContractAndCustomerInput) (cimodels.LoadContractAndCustomerOutput, error) {
	var (
		cust *customermodels.Customer
		cont *contractmodels.Contract
	)
	invokerID := intPtrToInt(in.TriggeredByUserID)
	customerOpts := customer.FreshOptions{InvokerUserID: invokerID, ConversationID: int(in.ConversationID), TraceID: in.TraceID}
	contractOpts := contract.FreshOptions{InvokerUserID: invokerID, ConversationID: int(in.ConversationID), TraceID: in.TraceID}

	g, gctx := errgroup.WithContext(ctx)
	if in.CustomerID != nil && *in.CustomerID != "" {
		customerID := *in.CustomerID
		g.Go(func() error {
			loaded, err := a.CustomerMgr.GetFreshByCustomerID(gctx, customerID, customerOpts)
			if errors.Is(err, customer.ErrCustomerNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			cust = loaded
			return nil
		})
	}
	if in.ContractID != nil && *in.ContractID != "" {
		contractID := *in.ContractID
		g.Go(func() error {
			loaded, err := a.ContractMgr.GetFreshByContractID(gctx, contractID, contractOpts)
			if errors.Is(err, contract.ErrContractNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			cont = loaded
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return cimodels.LoadContractAndCustomerOutput{}, err
	}

	if cust == nil && cont != nil && cont.CustomerID != "" {
		loaded, err := a.CustomerMgr.GetFreshByCustomerID(ctx, cont.CustomerID, customerOpts)
		if err != nil && !errors.Is(err, customer.ErrCustomerNotFound) {
			return cimodels.LoadContractAndCustomerOutput{}, err
		}
		cust = loaded
	}
	return cimodels.LoadContractAndCustomerOutput{Customer: cust, Contract: cont}, nil
}
