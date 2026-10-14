import type { Messages } from '../../types.ts';

const payables: Messages['payables'] = {
  docTitle: '仕入先買掛金 | ACIRABA',
  title: '仕入先への買掛金',
  subtitle: 'アクセスできる店舗で仕入先への掛け購入によって発生した買掛金です。分割払いが可能です。',
  search: '注文書番号、請求書番号、または仕入先を検索…',
  loadFailed: '買掛金一覧の読み込みに失敗しました。',
  empty: 'まだ買掛金はありません。',
  emptySearch: 'フィルターに一致する買掛金はありません。',
  loadMore: 'さらに読み込む',
  status: { open: '未払', overdue: '期日超過', paid: '支払済', all: 'すべて' },
  stat: { outstanding: '買掛金合計', overdue: '期日超過', open: '未払の注文書' },
  aging: { title: '買掛金の滞留', current: '期日前', d1_30: '1〜30日超過', d31_60: '31〜60日超過', d60_plus: '60日超過' },
  col: { docNo: '注文書番号', invoice: '請求書番号', supplier: '仕入先', date: '日付', due: '期日', amount: '買掛金', paid: '支払済', returned: '返品相殺', balance: '残高', status: 'ステータス', action: '操作' },
  noDue: '—',
  pay: '支払う',
  detail: '詳細',
  modal: {
    title: '買掛金 {doc}',
    history: '支払履歴',
    noPayments: 'まだ支払いはありません。',
    payTitle: '買掛金の支払い',
    method: '方法',
    amount: '支払額',
    ref: '参照番号',
    note: 'メモ',
    full: '全額を支払う',
    save: '支払いを保存',
    saving: '保存中…',
    saved: '支払い {doc} を保存しました。',
    settled: 'この買掛金は支払済みです。',
    paidBy: '支払者',
    afterPay: 'この支払い後の残高: {balance}',
    close: '閉じる'
  },
  history: {
    docTitle: '仕入価格履歴 | ACIRABA',
    title: '仕入価格履歴',
    subtitle: 'アクセスできる店舗で有効な各仕入注文書の商品の仕入価格です。新しい順に表示されます。',
    search: '商品名、コード、注文書番号、または仕入先を検索…',
    period: '期間',
    loadFailed: '仕入価格履歴の読み込みに失敗しました。',
    empty: 'この期間に仕入はありません。',
    loadMore: 'さらに読み込む',
    count: '{count} 行を読み込みました',
    col: { date: '日付', item: '商品', supplier: '仕入先', qty: '数量', price: '仕入価格', change: '変動', discount: '割引', cost: '行原価', doc: '注文書番号' },
    first: '先頭',
    preset: { today: '今日', days7: '7日', days30: '30日', month: '今月' }
  }
};

export default payables;
