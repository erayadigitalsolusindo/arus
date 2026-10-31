// Pilih opsi pertama (atau yang cocok dengan `text`) pada combobox bits-ui.
export async function pick(page, sel, text) {
  await page.locator(sel).click();
  const opts = page.locator('[role="option"]');
  await opts.first().waitFor({ timeout: 8000 });
  await (text ? opts.filter({ hasText: text }).first() : opts.first()).click();
  await page.waitForTimeout(250);
}
