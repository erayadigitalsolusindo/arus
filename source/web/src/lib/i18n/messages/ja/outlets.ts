import type { Messages } from '../../types.ts';

const outlets: Messages['outlets'] = {
  docTitle: '店舗 | ACIRABA',
  title: '店舗',
  subtitle: 'あなたのビジネスの支店または店舗を管理します。',
  add: '店舗を追加',
  edit: '店舗を編集',
  code: 'コード',
  codeHint: '小文字の英字、数字、- または _（2〜20文字）。作成後は変更できません。',
  name: '店舗名',
  timezone: 'タイムゾーン',
  taxStore: '店舗税 (%)',
  taxGov: '政府税 (%)',
  receipt: {
    title: 'レシート',
    hint: 'レジレシートに印刷されます（58mm紙、1行あたり約32文字）。不要な場合は空白のままにしてください。',
    address: '住所',
    phone: '電話番号',
    header: 'レシートヘッダーテキスト',
    footer: 'レシートフッターテキスト'
  },
  taxHint: '0〜100のパーセンテージ、小数点以下2桁まで。',
  status: 'ステータス',
  active: 'アクティブ',
  inactive: '非アクティブ',
  current: '使用中',
  save: '保存',
  saving: '保存中…',
  cancel: 'キャンセル',
  close: '閉じる',
  saved: '店舗を保存しました。',
  created: '店舗を追加しました。',
  empty: 'まだ店舗がありません。',
  loadFailed: '店舗データを読み込めませんでした。'
};
export default outlets;
