// Демо-витрина шаблона: товары и цены вымышлены, брендов нет. Нужна, пока у API нет каталога;
// данные кейса приедут с сервера, и этот файл уйдёт.
import type { Category, Product } from '@/lib/catalog'

export const DEMO_CATEGORIES: readonly Category[] = [
  { id: 'electronics', title: 'Электроника' },
  { id: 'home', title: 'Дом и кухня' },
  { id: 'sport', title: 'Спорт' },
  { id: 'clothes', title: 'Одежда' },
  { id: 'books', title: 'Книги' },
  { id: 'kids', title: 'Детям' },
]

export const DEMO_PRODUCTS: readonly Product[] = [
  {
    id: 'p01',
    title: 'Беспроводные наушники с шумоподавлением',
    category: 'electronics',
    price: 6990,
    oldPrice: 8490,
  },
  {
    id: 'p02',
    title: 'Портативная колонка с защитой от воды',
    category: 'electronics',
    price: 3490,
  },
  {
    id: 'p03',
    title: 'Зарядное устройство 65 Вт, два порта',
    category: 'electronics',
    price: 1990,
  },
  { id: 'p04', title: 'Чайник электрический стеклянный, 1,7 л', category: 'home', price: 2290 },
  {
    id: 'p05',
    title: 'Набор сковородок с антипригарным покрытием',
    category: 'home',
    price: 4590,
    oldPrice: 5990,
  },
  { id: 'p06', title: 'Плед из микрофибры 150 × 200 см', category: 'home', price: 1490 },
  { id: 'p07', title: 'Коврик для йоги, 6 мм', category: 'sport', price: 1290 },
  { id: 'p08', title: 'Гантели разборные, 2 × 10 кг', category: 'sport', price: 5490 },
  {
    id: 'p09',
    title: 'Кроссовки беговые лёгкие',
    category: 'clothes',
    price: 5990,
    oldPrice: 7490,
  },
  { id: 'p10', title: 'Худи из хлопка свободного кроя', category: 'clothes', price: 3290 },
  { id: 'p11', title: 'Сборник рассказов в твёрдой обложке', category: 'books', price: 690 },
  { id: 'p12', title: 'Алгоритмы для начинающих: учебник', category: 'books', price: 1190 },
  { id: 'p13', title: 'Конструктор «Город», 500 деталей', category: 'kids', price: 2990 },
  { id: 'p14', title: 'Настольная игра для всей семьи', category: 'kids', price: 1890 },
]
