// Страница товара: ссылка из выдачи, разбор rid/pos, выбор «назад» (src/lib/product.ts).
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  backTarget,
  compatStateText,
  descriptionText,
  durationText,
  isSearchPath,
  licenseRows,
  originText,
  parseProductRef,
  productHref,
  specText,
  termText,
} from '../src/lib/product.ts'

test('ссылка с rid и pos и без них', () => {
  assert.equal(productHref('kis-2dev-1y', '01JABC', 3), '/product/kis-2dev-1y?rid=01JABC&pos=3')
  assert.equal(productHref('kis-2dev-1y'), '/product/kis-2dev-1y')
  assert.equal(productHref('kis-2dev-1y', null, null), '/product/kis-2dev-1y')
  assert.equal(productHref('kis-2dev-1y', '01JABC'), '/product/kis-2dev-1y?rid=01JABC')
  // Неверные rid и pos в ссылку не попадают.
  assert.equal(productHref('x', 'плохой rid', 0), '/product/x')
  assert.equal(productHref('x', 'r', 10001), '/product/x?rid=r')
  assert.equal(productHref('x', 'r', 1.5), '/product/x?rid=r')
})

test('id экранируется', () => {
  assert.equal(productHref('a/b c?d'), '/product/a%2Fb%20c%3Fd')
  assert.equal(productHref('офис'), `/product/${encodeURIComponent('офис')}`)
})

test('разбор rid и pos: мусор — null, pos — целое 1..10000', () => {
  assert.deepEqual(parseProductRef({ rid: '01JABC', pos: '1' }), {
    requestId: '01JABC',
    position: 1,
  })
  assert.deepEqual(parseProductRef({ rid: 'r', pos: '10000' }), { requestId: 'r', position: 10000 })
  assert.deepEqual(parseProductRef({}), { requestId: null, position: null })
  for (const pos of ['0', '10001', '-1', '1.5', 'abc', '', ' 2', '1e3']) {
    assert.equal(parseProductRef({ pos }).position, null, pos)
  }
  for (const rid of ['', 'a b', '<script>', 'x'.repeat(65), 42]) {
    assert.equal(parseProductRef({ rid }).requestId, null, String(rid))
  }
  // Повтор параметра — берётся первый.
  assert.deepEqual(parseProductRef({ rid: ['a', 'b'], pos: ['2', '3'] }), {
    requestId: 'a',
    position: 2,
  })
})

test('«назад»: со страницы поиска — back, иначе /search', () => {
  assert.equal(backTarget('/search?q=касперский'), 'back')
  assert.equal(backTarget('/search'), 'back')
  assert.equal(backTarget('/search#x'), 'back')
  assert.equal(backTarget('/searchops'), '/search')
  assert.equal(backTarget('/product/x'), '/search')
  assert.equal(backTarget(null), '/search')
  assert.equal(backTarget(undefined), '/search')
  assert.equal(isSearchPath('/proofs'), false)
})

test('условия лицензии строками', () => {
  assert.deepEqual(
    licenseRows({
      type: 'subscription',
      metric: 'device',
      quantity: 2,
      duration_months: 12,
      edition: 'Standard',
    }),
    [
      { label: 'Тип', value: 'Подписка' },
      { label: 'Количество', value: '2 устройства' },
      { label: 'Срок', value: '1 год' },
      { label: 'Редакция', value: 'Standard' },
    ],
  )
  assert.deepEqual(
    licenseRows({
      type: 'perpetual',
      metric: 'org',
      quantity: null,
      duration_months: null,
      edition: '',
    }),
    [
      { label: 'Тип', value: 'Бессрочная' },
      { label: 'Метрика', value: 'На организацию' },
      { label: 'Срок', value: 'бессрочная' },
    ],
  )
  // Подписка без срока в данных: срок не выдумывается — строки «Срок» нет (было «бессрочная» у подписки).
  assert.deepEqual(
    licenseRows({
      type: 'subscription',
      metric: null,
      quantity: null,
      duration_months: null,
      edition: '',
    }),
    [{ label: 'Тип', value: 'Подписка' }],
  )
  assert.deepEqual(
    licenseRows({
      type: 'subscription',
      metric: 'device',
      quantity: 100,
      duration_months: 12,
      edition: '',
      tier_min: 100,
      tier_max: 149,
    })[1],
    { label: 'Количество', value: '100–149 устройств' },
  )
  assert.equal(termText('subscription', null), '')
  assert.equal(termText('renewal', 0), '')
  assert.equal(termText('perpetual', null), 'бессрочная')
  assert.equal(termText('subscription', 24), '2 года')
  assert.equal(durationText(18), '18 месяцев')
  assert.equal(durationText(36), '3 года')
})

test('совместимость, происхождение, единицы', () => {
  assert.equal(compatStateText('verified'), 'проверена')
  assert.equal(compatStateText('conditional'), 'с условием')
  assert.equal(compatStateText('incompatible'), 'несовместима')
  assert.equal(compatStateText('что-то'), 'неизвестно')
  assert.equal(originText('ru'), 'Отечественное')
  assert.equal(originText('foreign'), 'Зарубежное')
  assert.equal(originText(''), '')
  assert.equal(specText(64, 'ГБ'), '64 ГБ')
  assert.equal(specText(27, '″'), '27″')
  assert.equal(specText(2.5, ''), '2,5')
})

test('описание фида: пробел между склеенными предложениями', () => {
  assert.equal(
    descriptionText('для приложений.INFERIT RS212 имеет  диски U.2 NVMe.Задняя панель'),
    'для приложений. INFERIT RS212 имеет диски U.2 NVMe. Задняя панель',
  )
  assert.equal(descriptionText('Версия 1.8, Intel Core i5'), 'Версия 1.8, Intel Core i5')
})
