import type { Messages } from '../../types.ts';

const shell: Messages['shell'] = {
  openMenu: 'メニューを開く',
  closeMenu: 'メニューを閉じる',
  expandSidebar: 'サイドバーを展開',
  collapseSidebar: 'サイドバーを折りたたむ',
  tabs: 'ページタブ',
  closeTab: 'タブを閉じる',
  tabMenu: {
    close: 'タブを閉じる',
    closeOthers: '他のタブを閉じる',
    closeRight: '右側のタブを閉じる',
    closeAll: 'すべてのタブを閉じる'
  },
  unsaved: {
    title: '未保存の変更',
    body: 'このフォームは未保存で、タブを閉じると内容が失われます。変更を破棄しますか?',
    stay: 'このまま残す',
    discard: '変更を破棄',
    restored: '未保存の下書きが復元されました。このタブが開いている間は内容が保持されます。画像・写真は再選択する必要があります。'
  },
  mainNav: 'メインナビゲーション',
  sidebar: 'サイドバー',
  shortcuts: {
    title: 'ショートカット',
    target: '移動先',
    targetPh: '移動先メニューを選択…',
    customUrl: '他のアドレス（URL）…',
    manage: '管理',
    done: '完了',
    empty: 'まだショートカットがありません。',
    add: 'ショートカットを追加',
    edit: 'ショートカットを編集',
    remove: 'ショートカットを削除',
    name: '機能名',
    url: '機能URL',
    icon: 'アイコン',
    upload: 'アイコンをアップロード',
    removeIcon: 'アイコンを削除',
    iconHint: 'PNG、JPG、またはWebP、最大128KB。サーバーに保存され、他のデバイスでも利用できます。',
    iconInvalid: 'PNG、JPG、またはWebPの画像で、最大128KBのものを選択してください。',
    urlInvalid: '名前と有効なURL（アプリのパスまたはHTTP/HTTPSアドレス）を入力してください。',
    limit: 'ショートカットは最大8個までです。',
    loading: 'ショートカットを読み込み中…',
    loadFailed: 'サーバーからショートカットを読み込めませんでした。',
    saveFailed: 'アカウントにショートカットを保存できませんでした。',
    retry: 'もう一度試す'
  },
  notifications: '通知',
  noNotifications: 'まだ通知はありません。',
  profile: 'プロフィール',
  settings: 'エコシステム',
  ecosystem: {
    title: 'Erayaエコシステム',
    checking: '確認中',
    checkingDetails: 'このテナントのEraPayメンバーシップを確認中…',
    unknown: 'ステータス不明',
    active: 'アクティブ',
    inactive: '非アクティブ',
    intro: 'このビジネスをEraPayに接続します。同じテナントが2回作成されることはありません。',
    acceptTerms: 'EraPayの規約に同意し、このビジネスのマーチャントアカウント作成をリクエストします。',
    ownerInvite: '新しいアカウントにパスワード設定用のメールが届きます。このビジネスのオーナーメールにオーナーロールが付与されます。',
    activate: 'EraPayを有効化',
    activating: '接続中…',
    ownerConnected: 'このテナントはマーチャントオーナーとしてEraPayに接続されています。',
    reviewPending: 'EraPayマーチャントはメール確認とEraPayの承認を待っています。ここでの「アクティブ」はテナント接続が成功したことを意味します。',
    retry: 'もう一度試す'
  },
  signOut: 'ログアウト',
  currentOutlet: '現在の店舗',
  allOutlets: 'すべての店舗',
  allOutletsShort: 'すべて',
  switchOutlet: '店舗を切り替えますか?',
  server: {
    checking: 'サーバーを確認中…',
    online: 'サーバーオンライン',
    degraded: 'サーバーが不安定',
    offline: 'サーバーに到達できません'
  }
};

export default shell;
