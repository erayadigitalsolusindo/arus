import type { Messages } from '../../types.ts';

const settle: Messages['settle'] = {
  payable: { button: '仕入先ごとに支払う', title: '仕入先ごとの買掛金の支払い', party: '仕入先', partyHint: '仕入先を検索…', doc: '仕入注文書番号', empty: 'この仕入先には未払の買掛金がありません。' },
  receivable: { button: '会員ごとに受け取る', title: '会員ごとの支払いの受領', party: '会員', partyHint: '会員を検索…', doc: '売上伝票番号', empty: 'この会員には未払の売掛金がありません。' },
  mode: { auto: '自動（古い注文書から）', manual: '注文書を自分で選択' },
  autoHelp: '合計金額を入力してください。最も古い注文書から順に清算され、最後の注文書は一部だけ支払うことができます。',
  manualHelp: '支払う注文書にチェックを入れます。金額は残高の全額が入り、変更（分割払い）できます。',
  outstanding: '合計未払額: {amount}（{count} 件の注文書）',
  amount: '支払う合計金額',
  payAll: 'すべて支払う',
  selectAll: 'すべて選択',
  clear: 'クリア',
  col: { pick: '選択', date: '日付', due: '期日', balance: '残高', pay: '支払額', after: '最終残高' },
  preview: '資金の割り当て',
  previewEmpty: '各注文書への割り当てを確認するには金額を入力してください。',
  total: '合計: {amount}',
  method: '方法',
  ref: '参照番号',
  note: 'メモ',
  save: '清算を保存',
  saving: '保存中…',
  close: '閉じる',
  done: '清算 {doc} を保存しました',
  doneDetail: '{count} 件の注文書を支払いました（合計 {amount}）。',
  fee: '方法手数料 {fee}'
};

export default settle;
