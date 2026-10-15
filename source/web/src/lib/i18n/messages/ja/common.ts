import type { Messages } from '../../types.ts';

const common: Messages['common'] = {
  appName: 'ACIRABA',
  help: 'ヘルプ',
  language: '言語',
  theme: { light: 'ライトモード', dark: 'ダークモード' },
  paid: '支払済み',
  live: 'ライブ',
  close: '閉じる',
  dateRange: '日付範囲を選択',
  monthPicker: { choose: '月を選択', prev: '前の年', next: '次の年', thisMonth: '今月' },
  datePicker: { choose: '日付を選択', clear: 'クリア', today: '今日', prev: '前の月', next: '次の月' },
  markdown: {
    write: '書く',
    preview: 'プレビュー',
    empty: 'まだ内容がありません。',
    hint: 'Markdown対応: **太字**、*斜体*、リスト、リンク。生のHTMLは表示されません。'
  },
  errorPage: {
    notFoundTitle: '深海で迷子',
    notFoundBody: 'お探しのページが見つかりませんでした。移動したか、リンクが間違っている可能性があります。',
    serverTitle: '波が高い状態です',
    serverBody: 'サーバーでエラーが発生しました。しばらくしてからもう一度お試しください。',
    home: 'ホームに戻る',
    retry: '再読み込み'
  }
};

export default common;
