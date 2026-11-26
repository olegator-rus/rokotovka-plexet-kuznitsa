"""Контур доводки поверх готовых контуров поиска–3: закупка и честное «нет».

Вызывается из gen_mock.py последним, после контура выдачи. Данные контуров поиска–3 не меняются: позиции и семейства
дописываются в конец каталога (kind=software, популярность ниже любой позиции с событиями), связи — в конец
relations.json, журнал событий и наборы запросов не трогаются (test заморожен).

Что добавляется:
  * варианты Kaspersky Endpoint Security под тип покупки: продление на 1 и 2 года за узел (обе редакции),
    расширение (докупка узлов к действующей лицензии), миграция с другого антивируса, версия для образования;
  * тип организации в вариантах (attributes.org_type: education | government) — ограничение: такой вариант
    доступен только этой организации; тип покупки (attributes.deal: renewal | extension | migration) — вариант
    под конкретную сделку: показывается, только если запрос её называет (продление поштучно — с количеством
    или сроком);
  * семейство, которого не хватало для прогона витрины: КриптоПро CSP (электронная подпись). Новые семейства
    офисных пакетов не добавляются: неразмеченное семейство в выдаче test снижает NDCG замороженного набора;
  * reestr.json — статус в реестре российского ПО по семействам: confirmed (с номером записи) | not_matching;
    семейства без записи — «не установлено» (зарубежные — «не соответствует» по происхождению);
  * help.json — небольшой индекс справки магазина (счёт, документы, доставка, возврат, продление, активация);
  * showcase.json — 31 запрос прогона витрины, перенесённый на mock-каталог, с ожидаемым классом ответа.

Номера записей реестра в reestr.json синтетические (mock), как цены и артикулы: это не выписка из реестра.
"""

KIND = "software"
POPULARITY = 0.005  # ниже любой позиции с событиями: проверка популярности validate.py не меняется

VENDORS_P4 = [("cryptopro", "КриптоПро", "ru")]

KES_DESC = ("Корпоративная защита рабочих станций и серверов с централизованным управлением через Kaspersky "
            "Security Center. Поддерживает Windows, Linux (включая Astra Linux и РЕД ОС) и macOS.")
KES_ATTRS = {"platform": "Windows, Linux, macOS"}

# id, редакция, тип лицензии, срок (мес.), цена за узел, хвост названия, пояснение, доп. атрибуты
KES_VARIANTS = [
    ("kes-std-ren12", "Стандартный", "renewal", 12, 2390, "продление на 1 год, за 1 узел",
     "Продление действующей лицензии: число узлов указывается при заказе.", {"deal": "renewal"}),
    ("kes-std-ren24", "Стандартный", "renewal", 24, 4290, "продление на 2 года, за 1 узел",
     "Продление действующей лицензии на два года: число узлов указывается при заказе.", {"deal": "renewal"}),
    ("kes-ext-ren12", "Расширенный", "renewal", 12, 3390, "продление на 1 год, за 1 узел",
     "Продление действующей лицензии: число узлов указывается при заказе.", {"deal": "renewal"}),
    ("kes-ext-ren24", "Расширенный", "renewal", 24, 6090, "продление на 2 года, за 1 узел",
     "Продление действующей лицензии на два года: число узлов указывается при заказе.", {"deal": "renewal"}),
    ("kes-std-add12", "Стандартный", "subscription", 12, 2990, "расширение лицензии, за 1 узел",
     "Расширение: докупка узлов к действующей лицензии до конца её срока.", {"deal": "extension"}),
    ("kes-std-mig12", "Стандартный", "subscription", 12, 1990, "миграция с другого антивируса, за 1 узел, 1 год",
     "Миграция: переход с антивируса другого производителя со скидкой при сдаче действующей лицензии.",
     {"deal": "migration"}),
    ("kes-ext-mig12", "Расширенный", "subscription", 12, 2890, "миграция с другого антивируса, за 1 узел, 1 год",
     "Миграция: переход с антивируса другого производителя со скидкой при сдаче действующей лицензии.",
     {"deal": "migration"}),
    ("kes-std-edu12", "Стандартный", "subscription", 12, 1290, "для образования, за 1 узел, 1 год",
     "Лицензия для образовательных учреждений: школ, колледжей и вузов.", {"org_type": "education"}),
]

DRWEB_GOV = ("dress-gov12", "Комплексная защита", "subscription", 12, 1990,
             "для государственных учреждений, за 1 узел, 1 год",
             "Лицензия для государственных и муниципальных учреждений.", {"org_type": "government"})

MYOFFICE_EDU = dict(
    id="myo-edu1", title="МойОфис Стандартный для образования, 1 пользователь, 1 год",
    extra="Лицензия для образовательных учреждений: школ, колледжей и вузов.",
    license=dict(type="subscription", metric="user", quantity=1, duration_months=12, edition=None), price=990,
    attrs={"org_type": "education"})

NEW_FAMILIES = [
    dict(family_id="cryptopro-csp", family_name="КриптоПро CSP 5.0", vendor_id="cryptopro", category_id="infosec",
         desc="Криптопровайдер для электронной подписи и шифрования. Нужен для работы с квалифицированной "
              "электронной подписью, электронным документооборотом и госпорталами. Сертифицирован ФСБ России.",
         attrs={"platform": "Windows, Linux, macOS"},
         variants=[
             ("cpcsp-perp", "КриптоПро CSP 5.0, лицензия на рабочее место, бессрочная", "perpetual", "device", 1,
              None, 2700, True),
             ("cpcsp-1y", "КриптоПро CSP 5.0, лицензия на рабочее место, 1 год", "subscription", "device", 1, 12,
              1500, True),
             ("cpcsp-srv", "КриптоПро CSP 5.0, серверная лицензия, бессрочная", "perpetual", "server", 1, None,
              37500, False),
         ]),
]

NEW_RELATIONS = [
    # (from, to, type, note, from_product, to_product, upgrade_adds)
    ("kaspersky-endpoint-security", "kaspersky-endpoint-security", "upgrade", "Расширенный вместо Стандартного",
     "kes-std-ren12", "kes-ext-ren12", ["Шифрование дисков и файлов", "Управление исправлениями ПО"]),
    ("kaspersky-endpoint-security", "kaspersky-endpoint-security", "upgrade", "Расширенный вместо Стандартного",
     "kes-std-ren24", "kes-ext-ren24", ["Шифрование дисков и файлов", "Управление исправлениями ПО"]),
]

# Алиасы из прогона витрины (п. 6). «др веб», «ворд», «винда», «1c» уже есть в словаре контура поиска;
# «скуль» намеренно не добавляется — это заложенный пробел агента (regression alias_gap «скуль сервер 2022»).
# «сертификат» → «сертифицирован»: в каталоге «Сертифицирована ФСТЭК», а слово запроса модель разбирает по-разному
# (витрина №19 «сертификат фстэк»).
ALIASES_P4 = [
    ("сертификат", "сертифицирован", "alias", "term", None),
    ("сертификатом", "сертифицирован", "alias", "term", None),
]

# Реестр российского ПО: семейство → номер записи (синтетический, mock). Остальные отечественные — не установлено
# (в том числе Р7-Офис: «нет подтверждения в данных», а не «нет в реестре» — так в макете).
REESTR_CONFIRMED = [
    ("myoffice-standard", "1083"), ("astra-linux-se", "1311"),
    ("red-os", "1422"), ("alt-os", "1535"), ("kaspersky-endpoint-security", "1646"),
    ("kaspersky-small-office-security", "1757"), ("drweb-enterprise-security-suite", "1868"),
    ("drweb-security-space", "1979"), ("kompas-3d", "2081"), ("nanocad", "2192"), ("postgres-pro", "2303"),
    ("cyber-backup", "2414"), ("trueconf-server", "2525"), ("secret-net-studio", "2636"),
    ("1c-accounting", "2747"), ("1c-client-license", "2858"), ("cryptopro-csp", "2969"),
]
REESTR_CHECKED = "2026-05-22"

HELP = [
    dict(id="invoice", title="Как получить счёт на оплату",
         text="Счёт формируется при оформлении заказа: выберите «Оплата по счёту» и укажите реквизиты организации. "
              "Счёт придёт на почту и появится в кабинете в разделе «Заказы».",
         link_label="Заказы и счета в кабинете", url="/help#invoice",
         triggers=["счет", "счёт", "счета", "счету", "счетом", "оплатить по счету", "реквизиты", "оплата", "оплатить"]),
    dict(id="documents", title="Закрывающие документы",
         text="УПД, акты и счета-фактуры по оплаченным заказам доступны в кабинете в разделе «Документы»; "
              "оригиналы отправляются через ЭДО или почтой.",
         link_label="Документы в кабинете", url="/help#documents",
         triggers=["документы", "закрывающие документы", "закрывающие", "упд", "акт", "акт сверки", "счет-фактура",
                   "счет фактура", "эдо"]),
    dict(id="delivery", title="Доставка",
         text="Электронные лицензии приходят на почту после оплаты, обычно в течение рабочего дня. "
              "Коробки и оборудование доставляются курьером; срок зависит от города и наличия.",
         link_label="Условия доставки", url="/help#delivery",
         triggers=["доставка", "доставки", "доставку", "доставить", "привезти", "курьер", "самовывоз", "где мой заказ"]),
    dict(id="return", title="Возврат",
         text="Электронную лицензию можно вернуть до активации ключа; оборудование — в течение 14 дней в "
              "заводской упаковке. Заявка на возврат оформляется в кабинете.",
         link_label="Как оформить возврат", url="/help#return",
         triggers=["возврат", "возврата", "вернуть", "вернуть деньги", "обмен"]),
    dict(id="renewal", title="Как продлить лицензию",
         text="Найдите продукт и выберите вариант «Продление» с нужным сроком и числом узлов — или откройте "
              "кабинет: в разделе «Лицензии» у каждой лицензии есть кнопка «Продлить».",
         link_label="Как продлить в кабинете", url="/help#renewal",
         triggers=["как продлить", "продлить лицензию", "продление лицензии", "срок лицензии", "лицензия истекает",
                   "истекает лицензия"], mixed="renewal"),
    dict(id="activation", title="Активация ключа",
         text="Ключ и инструкция приходят на почту после оплаты и хранятся в кабинете в разделе «Лицензии». "
              "Если ключ не принимается — напишите в поддержку с номером заказа.",
         link_label="Ключи в кабинете", url="/help#activation",
         triggers=["активация", "активировать", "активации", "ключ активации", "как получить ключ", "ключ не подходит",
                   "не активируется"]),
]

# Прогон витрины, перенесённый на
# mock-каталог: (№, запрос, оценка витрины, ожидаемый класс на mock, пояснение).
SHOWCASE = [
    (1, "др веб", "fail", "product", "алиас «др веб» → Dr.Web"),
    (2, "dr.web", "ok", "product", ""),
    (3, "ворд", "fail", "product", "алиас «ворд» → Word (контур поиска); офисные пакеты Microsoft"),
    (4, "мойофис", "ok", "product", ""),
    (5, "р7 офис", "ok", "product", ""),
    (6, "microsoft office", "ok", "product", ""),
    (7, "windows server 2025", "fail", "product", "на витрине Windows Server нет; в mock — есть"),
    (8, "1с", "partial", "product", "сама 1С в первой тройке"),
    (9, "1c бухгалтерия", "ok", "product", ""),
    (10, "битрикс24", "ok", "absent", "в mock-каталоге Битрикс24 нет — честное «нет»"),
    (11, "астра линукс", "partial", "product", "Astra Linux первой"),
    (12, "антивирус для сервера", "partial", "product", ""),
    (13, "резервное копирование", "ok", "product", ""),
    (14, "видеоконференции", "partial", "product", ""),
    (15, "электронная подпись", "fail", "product", "КриптоПро CSP, без шума"),
    (16, "криптопро", "ok", "product", ""),
    (17, "аналог microsoft office", "fail", "analogs", "отечественные офисные пакеты, исходный — «Вы искали»"),
    (18, "отечественная субд", "partial", "product", "Postgres Pro, без ОС"),
    (19, "сертификат фстэк", "partial", "product", "позиции с сертификатом ФСТЭК"),
    (20, "для образования", "unknown", "product", "варианты для образовательных учреждений"),
    (21, "продление касперский", "fail", "product", "продления Kaspersky, KES в первой пятёрке"),
    (22, "касперский на 50 пользователей", "fail", "product", "Kaspersky на 50, метрика устройств — с пояснением"),
    (23, "kaspersky на 3 года", "partial", "product", "срока 3 года нет — сказано прямо"),
    (24, "сервер 2u", "partial", "product", ""),
    (25, "ноутбук 16 гб", "fail", "product", "ноутбуки с 16 ГБ"),
    (26, "коммутатор 24 порта", "ok", "product", ""),
    (27, "монитор 27", "ok", "product", ""),
    (28, "счёт", "fail", "help", "ответ справки без товаров"),
    (29, "доставка", "fail", "help", "ответ справки без товаров"),
    (30, "возврат", "fail", "help", "ответ справки без товаров"),
    (31, "как продлить лицензию", "fail", "help", "ответ справки без товаров"),
]


def _kes_product(v, by_id, vendors_by_id, base_kes):
    pid, edition, ltype, months, price, tail, extra, attrs = v
    a = dict(KES_ATTRS)
    a.update({"edition": edition, "purchase": "поштучно"})
    a.update(attrs)
    lic_word = "продление" if ltype == "renewal" else "подписка"
    term = "1 год" if months == 12 else "2 года"
    desc = f"{KES_DESC} {extra} Лицензия: {lic_word} на {term}; за 1 узел, количество указывается при заказе."
    return _product(dict(id=pid, family_id=base_kes["family_id"], family_name=base_kes["family_name"],
                         title=f"{base_kes['family_name']} {edition}, {tail}", vendor_id="kaspersky",
                         category_id=base_kes["category_id"], description=desc, attributes=a,
                         license=dict(type=ltype, metric="device", quantity=1, duration_months=months,
                                      edition=edition), price=price), vendors_by_id)


def _product(b, vendors_by_id):
    origin = vendors_by_id[b["vendor_id"]]["origin"]
    return {
        "id": b["id"], "sku": "", "family_id": b["family_id"], "family_name": b["family_name"], "title": b["title"],
        "vendor_id": b["vendor_id"], "category_id": b["category_id"], "description": b["description"],
        "attributes": b["attributes"], "license": b["license"], "price_rub": b["price"],
        "price_on_request": b["price"] is None, "origin": origin, "in_reestr": origin == "ru",
        "popularity": POPULARITY, "margin": 0.15, "kind": KIND, "mpn": "", "specs": {}, "compatible_with": [],
        "model_id": None, "price_mode": "fixed" if b["price"] is not None else "quote_required",
    }


def build_products(products, vendors_by_id):
    by_id = {p["id"]: p for p in products}
    base_kes = by_id["kes-std10"]
    out = [_kes_product(v, by_id, vendors_by_id, base_kes) for v in KES_VARIANTS]
    dr = by_id["dress-cx10"]
    pid, edition, ltype, months, price, tail, extra, attrs = DRWEB_GOV
    a = dict(dr["attributes"])
    a.update({"edition": edition, "purchase": "поштучно"})
    a.update(attrs)
    out.append(_product(dict(
        id=pid, family_id=dr["family_id"], family_name=dr["family_name"], title=f"{dr['family_name']} {edition}, {tail}",
        vendor_id=dr["vendor_id"], category_id=dr["category_id"],
        description=f"{dr['description'].split(' Лицензия:')[0]} {extra} Лицензия: подписка на 1 год; за 1 узел, "
                    f"количество указывается при заказе.",
        attributes=a, license=dict(type=ltype, metric="device", quantity=1, duration_months=months, edition=edition),
        price=price), vendors_by_id))
    myo = by_id["myo-s1"]
    a = dict(myo["attributes"])
    a.update(MYOFFICE_EDU["attrs"])
    out.append(_product(dict(
        id=MYOFFICE_EDU["id"], family_id=myo["family_id"], family_name=myo["family_name"], title=MYOFFICE_EDU["title"],
        vendor_id=myo["vendor_id"], category_id=myo["category_id"],
        description=f"{myo['description'].split(' Лицензия:')[0]} {MYOFFICE_EDU['extra']} Лицензия: подписка на "
                    f"1 год; 1 пользователь; продаётся поштучно, количество указывается при заказе.",
        attributes=a, license=MYOFFICE_EDU["license"], price=MYOFFICE_EDU["price"]), vendors_by_id))
    for f in NEW_FAMILIES:
        for pid, title, ltype, metric, qty, months, price, by_one in f["variants"]:
            a = dict(f["attrs"])
            if by_one:
                a["purchase"] = "поштучно"
            term = "бессрочная" if months is None else "подписка на 1 год"
            unit = {"device": "1 рабочее место", "user": "1 пользователь", "server": "1 сервер"}[metric]
            out.append(_product(dict(
                id=pid, family_id=f["family_id"], family_name=f["family_name"], title=title, vendor_id=f["vendor_id"],
                category_id=f["category_id"], description=f"{f['desc']} Лицензия: {term}; {unit}.", attributes=a,
                license=dict(type=ltype, metric=metric, quantity=qty, duration_months=months, edition=None),
                price=price), vendors_by_id))
    ids = [p["id"] for p in out]
    assert not set(ids) & set(by_id), "p4: повтор id позиции"
    return out


def build_relations(relations, rel_factory):
    out = list(relations)
    for frm, to, typ, note, fp, tp, adds in NEW_RELATIONS:
        out.append(rel_factory(f"rel-{len(out) + 1:04d}", frm, to, typ, note, compatibility="verified",
                               source="vendor_documentation" if typ == "upgrade" else "catalog_rule",
                               evidence_level="E2" if typ == "upgrade" else "E3", reviewed_at="2026-05-27",
                               from_product=fp, to_product=tp, upgrade_adds=adds,
                               quantity_strategy="per_device" if typ == "upgrade" else None))
    return out


def build_reestr(products):
    fams = {p["family_id"] for p in products}
    out = []
    for fid, number in REESTR_CONFIRMED:
        assert fid in fams, f"reestr: нет семейства {fid}"
        out.append({"family_id": fid, "status": "confirmed", "number": number, "checked_at": REESTR_CHECKED,
                    "source": "mock"})
    return out


def build_showcase():
    return [{"id": f"sc-{n:02d}", "query": q, "vitrine": v, "expect": e, "note": note} for n, q, v, e, note in SHOWCASE]


def extend(products, vendors, relations, aliases, rel_factory):
    """Дописывает контур доводки. Возвращает (relations, extra_files: {имя: данные})."""
    have = {a["alias"] for a in aliases}
    aliases += [dict(alias=a, canonical=c, kind=k, target=t, target_id=tid) for a, c, k, t, tid in ALIASES_P4
                if a not in have]
    vendors += [dict(id=i, name=n, origin=o) for i, n, o in VENDORS_P4]
    vendors_by_id = {v["id"]: v for v in vendors}
    products += build_products(products, vendors_by_id)
    rels = build_relations(relations, rel_factory)
    extra = {
        "reestr.json": build_reestr(products),
        "help.json": HELP,
        "showcase.json": build_showcase(),
    }
    return rels, extra
