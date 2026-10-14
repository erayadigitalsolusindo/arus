import type { Messages } from '../../types.ts';

const deposits: Messages['deposits'] = {
  title: 'デポジット',
  balance: 'デポジット残高',
  hint: '会員が店に預けるお金です。売上の支払いや売掛金の支払いに使えます。売上返品の返金もここに入ります。',
  topup: '入金',
  withdraw: '引出し',
  empty: 'まだデポジット履歴はありません。',
  loadMore: 'さらに読み込む',
  form: {
    topupTitle: 'デポジット入金',
    withdrawTitle: 'デポジット引出し',
    topupHint: '会員が店にお金を預けます（アクティブな店舗のレジ/口座に入金されます）。',
    withdrawHint: '会員がデポジットの一部または全額を受け取ります（アクティブな店舗から出金されます）。',
    amount: '金額',
    method: '方法',
    ref: '参照番号',
    refHint: '振込、デビット、カード、eウォレットでは必須です。',
    note: 'メモ（任意）',
    save: '保存',
    saving: '保存中…',
    saved: '文書 {doc} を保存しました。',
    cancel: 'キャンセル'
  },
  history: { time: '時刻', kind: '種別', doc: '文書', amount: '金額', balance: '残高', by: '担当者' },
  kind: {
    TOPUP: '入金',
    WITHDRAW: '引出し',
    SALE_PAYMENT: '売上の支払い',
    SALE_REVERSAL: '伝票の取消/変更',
    RECEIVABLE_PAYMENT: '売掛金の支払い',
    SALE_RETURN: '返金',
    SALE_RETURN_VOID: '返品の取消',
    PURCHASE_RETURN: '仕入返品の返金',
    PURCHASE_RETURN_VOID: '仕入返品の取消',
    PAYABLE_PAYMENT: '買掛金の支払い',
    CASH_OUT: '現金化'
  }
};

export default deposits;
