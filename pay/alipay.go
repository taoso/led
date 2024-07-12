package pay

import (
	"context"
	"fmt"
	"net/http"

	"github.com/smartwalle/alipay/v3"
)

type Alipay struct {
	client *alipay.Client
}

// New 创建支付宝实例
func New(appID, privateKey, publicKey string) *Alipay {
	client, err := alipay.New(appID, privateKey, true)
	if err != nil {
		panic(err)
	}
	if err = client.LoadAliPayPublicKey(publicKey); err != nil {
		panic(err)
	}
	return &Alipay{client: client}
}

type Order struct {
	Subject string
	TradeNo string
	Amount  string
	Extra   string

	NotifyURL string
}

// CreateQR 创建二维码支付订单
func (ali *Alipay) CreateQR(o Order) (string, error) {
	r, err := ali.client.TradePreCreate(context.TODO(), alipay.TradePreCreate{
		Trade: alipay.Trade{
			NotifyURL:      o.NotifyURL,
			Subject:        o.Subject,
			OutTradeNo:     o.TradeNo,
			TotalAmount:    o.Amount,
			PassbackParams: o.Extra,
			TimeoutExpress: "15m",
		},
	})
	if err != nil {
		return "", err
	}

	if r.Code != alipay.CodeSuccess {
		return "", fmt.Errorf("TradePreCreate error: %w", err)
	}

	return r.QRCode, nil
}

func (ali *Alipay) GetNotification(req *http.Request) (n *alipay.Notification, err error) {
	n, err = ali.client.GetTradeNotification(req)
	return
}
