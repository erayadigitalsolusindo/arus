import type { Messages } from '../../types.ts';

const supplierCredits: Messages['supplierCredits'] = {
  docTitle: '仕入先クレジット | ACIRABA',
  title: '仕入先クレジット',
  subtitle: '仕入返品の返金に基づき、店舗がまだ保有している仕入先の資金です。買掛金の支払いに充てることも、仕入先が現金化することもできます。',
  search: '仕入先を検索…',
  onlyPositive: '残高のあるもののみ',
  col: { supplier: '仕入先', balance: 'クレジット残高' },
  empty: '仕入先クレジットはまだありません。',
  loadMore: 'さらに読み込む',
  loadFailed: '仕入先クレジットの読み込みに失敗しました。',
  balanceLine: '仕入先クレジット残高: {amount}',
  over: 'クレジット残高を超えています',
  detail: '詳細',
  cashOut: {
    action: '現金化を記録',
    title: '仕入先クレジットの現金化',
    hint: '仕入先が残りのクレジットを店舗に支払います（資金は有効な店舗に入ります）。'
  }
};

export default supplierCredits;
