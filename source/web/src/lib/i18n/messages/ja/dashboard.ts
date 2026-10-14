import type { Messages } from '../../types.ts';

const dashboard: Messages['dashboard'] = {
  docTitle: 'ダッシュボード · ACIRABA',
  live: {
    title: 'ライブ売上',
    subtitle: '本日 · {outlet}',
    statusLive: 'LIVE',
    statusConnecting: '接続中…',
    statusOffline: '接続が切れました。再接続中…',
    failed: '最新データを読み込めませんでした。最後のデータを表示しています。',
    sales: '本日の売上',
    receipts: 'レシート数',
    average: '1レシートあたりの平均',
    returns: '本日の返品',
    net: '純額: {amount}',
    vsYesterday: '昨日の同時刻との比較',
    noBaseline: '昨日は売上がありません',
    perHour: '時間別売上',
    recent: '最新の取引',
    empty: '本日の売上はまだありません。',
    walkIn: '一般',
    lines: { one: '{count} 点', other: '{count} 点' },
    updated: '{time} 更新'
  },
  title: '店舗サマリー',
  refresh: '更新',
  updatedAt: '{time} 更新',
  loadError: 'サマリーを読み込めませんでした。接続を確認して、もう一度お試しください。',
  retry: '再試行',
  noAccess: 'このアカウントにはサマリーを表示する権限がありません。店舗オーナーにご連絡ください。',
  scope: { outlet: '店舗 {name}', all: '全店舗 ({count})' },
  verdict: {
    ok: { title: '店舗は正常です', text: 'すべてのチェックが完了しました。今すぐ対応が必要なものはありません。' },
    warn: { title: '確認すべき点があります', text: '店舗は稼働していますが、{count} 件を確認することをおすすめします。' },
    bad: { title: '早急な対応が必要です', text: '{count} 件に重大な問題があります。下のリストをご確認ください。' }
  },
  checksTitle: '確認した項目',
  checksSub: '緑は安全、黄は要確認、赤は早急な対応が必要です。',
  open: '表示',
  check: {
    sales_pace: {
      ok: '本日の売上は同時刻の通常時と比べて {pct} です — 正常です。',
      warn: '本日の売上は同時刻の通常時と比べて {pct} です。レジが開いているか、いつもどおり繁盛しているかご確認ください。'
    },
    sales_pace_nodata: { ok: '4週間の履歴が十分にないため、売上を比較できません — データが揃うと自動で表示されます。' },
    voids: {
      ok: '本日取り消されたレシート: {count} — 正常です。',
      warn: '本日 {count} 件のレシートが取り消されており、通常より多めです。売上リストで理由をご確認ください。',
      bad: '本日 {count} 件のレシートが取り消されています — かなり多いです。売上リストで早急にご確認ください。'
    },
    shift_open_long: {
      ok: '閉め忘れたレジシフトはありません。',
      warn: '{count} 件のレジシフトが18時間以上開いたままです — 閉め忘れの可能性があります。'
    },
    shift_diff: {
      ok: '過去7日間、レジの現金は記録と一致しています。',
      warn: '過去7日間に {count} 件のシフトで現金差額がありました（合計 {amount}）。'
    },
    stock_low_unset: { ok: '最小在庫数がまだ設定されていません — 商品フォームで設定すると、在庫が減った際に警告されます。' },
    stock_low: {
      ok: '在庫が少なくなっている商品（最小在庫数を下回った商品）はありません。',
      warn: '{count} 点の商品の在庫が少なくなっています — 最小在庫数を下回っているため、仕入れの時期です。'
    },
    stock_negative: {
      ok: 'マイナスの在庫はありません。',
      warn: '{count} 点の商品の在庫がマイナスです — 通常、入庫を記録する前に売れてしまったものです。'
    },
    receivable_overdue: {
      ok: '支払期日を過ぎたメンバー売掛金はありません。',
      warn: 'メンバー売掛金が {amount} 分、期日を過ぎています。',
      bad: 'メンバー売掛金の半分以上（{amount}）が期日を過ぎています。'
    },
    payable_overdue: {
      ok: '支払期日を過ぎた仕入先買掛金はありません。',
      warn: '仕入先買掛金が {amount} 分、期日を過ぎています。',
      bad: '仕入先買掛金が30日以上、期日を過ぎています（{amount}）。'
    }
  },
  kpi: {
    salesToday: '本日の売上',
    yesterday: '昨日（終日）: {amount}',
    vsUsual: '通常時との比較 {pct}',
    vsUsualNone: '比較対象なし',
    vsUsualHint: '過去4週間の同じ曜日の同時刻までの平均と比較しています。',
    receipts: 'レシート数',
    average: '1レシートあたり平均 {amount}',
    cancelled: '{count} 件取り消し',
    profit: '本日の粗利',
    profitHint: '原価と返品を差し引いた後、税およびその他費用の前です。',
    week: '過去7日間',
    month: '今月',
    monthVs: '先月との比較 {pct}',
    monthVsHint: '先月の同じ日付・同時刻との比較: {amount}。',
    monthVsNone: '先月は売上がありません',
    returns: '本日の返品: {count} 件（{amount}）'
  },
  chart: {
    trend: '日別売上',
    days: '{count} 日',
    hourly: '本日の時間別売上',
    peak: '最も忙しいのは {hour} 時',
    today: '本日',
    receiptsCount: '{count} 件のレシート',
    noSales: 'この期間の売上はまだありません。レジで販売を開始するとグラフが表示されます。'
  },
  top: { title: 'ベストセラー', sub: '過去7日間、売上別', qty: '{qty} 点販売', empty: 'まだ売れた商品はありません。' },
  stock: {
    title: '在庫状況',
    tracked: '残高のある商品',
    empty: '在庫切れ',
    negative: 'マイナス在庫',
    low: '少なめ',
    lowList: '最も少ないもの（在庫 / 最小在庫数）',
    lowUnset: '商品フォームで「最小在庫数」を設定すると、在庫が少ない商品がここに表示されます。',
    value: '在庫額',
    valueHint: '数量 × 平均原価。',
    all: '商品リストを表示',
    negativeList: '最大のマイナス在庫'
  },
  receivable: { title: 'メンバー売掛金', outstanding: '未払い', overdue: '期日超過', open: '{count} 件の未払いレシート', all: '売掛金リストを表示' },
  payable: { title: '仕入先買掛金', outstanding: '未払い', overdue: '期日超過', open: '{count} 件の未払い請求書', all: '買掛金リストを表示' },
  branches: {
    title: '店舗比較',
    sub: '各店舗の現在の売上と状況',
    outlet: '店舗',
    today: '本日',
    receipts: 'レシート',
    yesterday: '昨日',
    week: '7日間',
    stock: '在庫',
    shifts: '開いているシフト',
    stockOk: '正常',
    negative: '{count} 件マイナス',
    low: '{count} 件少なめ',
    empty: '{count} 件在庫切れ'
  },
  wide: '売掛金と買掛金は、アクセスできるすべての店舗を常にカバーします。',
  shifts: {
    title: '現金とレジシフト',
    running: '進行中のシフト',
    none: '現在開いているシフトはありません。',
    since: '{time} から',
    long: '長すぎ',
    diff: '過去7日間の現金差額',
    noDiff: '差額なし',
    closed: '今週閉じたシフト {count} 件',
    all: 'シフトリストを表示'
  }
};
export default dashboard;
