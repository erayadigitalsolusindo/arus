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
  geo: {
    title: '位置情報（GPS）',
    hint: 'ACIRABAチームが店舗の場所を把握するための位置情報です。店舗にいるときにボタンを押してください。',
    lat: '緯度',
    lng: '経度',
    use: '現在地を取得',
    locating: '現在地を取得中…',
    clear: '位置情報を削除',
    open: '地図で見る',
    denied: '位置情報の許可が拒否されました。ブラウザで位置情報を許可して再試行してください。',
    unavailable: 'このデバイスでは位置情報を利用できません。',
    invalid: '緯度（−90〜90）と経度（−180〜180）を両方入力するか、両方空にしてください。'
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
