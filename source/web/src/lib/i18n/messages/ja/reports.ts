import type { Messages } from '../../types.ts';

const reports: Messages['reports'] = {
  docTitle: '日付別売上レポート',
  title: '日付別売上',
  subtitle: '日次集計と支払方法別内訳',
  range: '日付範囲',
  hideEmpty: '売上のない日を隠す',
  date: '日付',
  saleCount: '伝票数',
  voidCount: 'キャンセル',
  subtotal: '小計',
  discount: '割引',
  taxStore: '店舗税',
  taxGov: '国税',
  total: '合計',
  paid: '受領額',
  receivable: '売掛金',
  voidTotal: 'キャンセル額',
  methods: '支払方法',
  methodName: '名称',
  methodKind: '種類',
  methodAmount: '金額',
  methodFee: 'MDR料',
  methodSurcharge: '附加料金',
  returns: '返品',
  returnCount: '返品数',
  returnTotal: '返品額',
  netTotal: '净额',
  noData: 'この範囲にデータがありません。',
};

export default reports;
