import type { Messages } from '../../types.ts';

const onboarding: Messages['onboarding'] = {
  docTitle: '初期設定 | ACIRABA',
  title: 'ARUS をセットアップ',
  subtitle: 'アプリをすぐ使えるようにするための簡単な手順です。',
  skip: 'セットアップをスキップ',
  back: '戻る',
  next: '次へ',
  stepOf: 'ステップ {n} / {total}',
  steps: { email: 'メール', coa: '会計', ready: '準備状況', done: '完了' },
  email: {
    title: 'メールアドレスの確認',
    text: '{email} に確認リンクを送信しました。どの端末で開いても構いません。このページは自動で更新されます。',
    verified: 'メール {email} は確認済みです。',
    waiting: '確認を待っています…',
    resend: 'メールを再送',
    sending: '送信中…',
    sent: 'メールを送信しました。受信トレイと迷惑メールをご確認ください。',
    later: '後で確認して先に進む'
  },
  coa: {
    title: '会計テンプレートを導入',
    text: '小売向けの勘定科目表（資産、負債、純資産、収益、売上原価、費用）、自動仕訳の対応付け、支払方法用の現金/銀行口座がすぐに使えます。',
    point1: '売上・仕入・在庫・現金の仕訳が自動で作成されます',
    point2: '損益計算書、貸借対照表、総勘定元帳がすぐに表示されます',
    point3: '勘定科目と対応付けはいつでも変更できます',
    install: '小売テンプレートを導入',
    installing: '導入中…',
    installed: '会計テンプレートを導入しました（{count} 科目）。',
    failed: 'テンプレートを導入できませんでした。再試行するか、会計メニューで設定してください。',
    noPermission: '勘定科目を作成できる権限のあるユーザーのみ導入できます。'
  },
  ready: {
    title: '店舗の準備を確認',
    text: '現在のデータに基づく確認です。',
    outlet: '最初の店舗を作成済み',
    methods: '支払方法が現金/銀行口座に紐づいている',
    methodsFix: '{count} 件の支払方法が口座に紐づいていません。支払方法で設定してください。',
    coa: '勘定科目表を導入済み',
    items: '最初の商品を追加済み',
    itemsHint: 'レジで販売できるよう商品を追加してください。今でも後でも構いません。',
    addItems: '商品を追加',
    fixMethods: '支払方法を開く'
  },
  done: {
    title: '準備完了',
    text: 'ARUS のセットアップが完了しました。設定はいつでもメニューから変更できます。',
    start: 'ARUS を使い始める',
    saving: '保存中…',
    failed: '保存できませんでした。再試行してください。'
  }
};

export default onboarding;
