"""Кластеры запросов (часть 1): помощники разметки, demo, точные названия, артикулы.

Кластер — одно намерение и его формулировки (точное имя, опечатка, раскладка, сленг). Разметка задаётся
на уровне кластера (j) и может уточняться у запроса. Все запросы кластера попадают в один набор.

J(grade, **filters) — правило разметки: grade получают все позиции каталога, подходящие под фильтры;
если позиция подходит под несколько правил, берётся максимальная оценка.
opt=True — правило может ни с чем не совпасть.
Фильтры: fam (id семейства или список), ids, not_ids, cat, lt / not_lt (тип лицензии), metric, qty, mo (срок,
None — бессрочно), ed (подстрока редакции), origin, unit (продаётся поштучно).
Запрос: (type, text, freq[, opts]); opts: n — заметка, j — своя разметка, c — client_id,
r — кластер, куда пользователь переформулирует запрос (для логов; по умолчанию reform кластера).
"""

REN = ["renewal"]

_EN = "qwertyuiop[]asdfghjkl;'zxcvbnm,.`"
_RU = "йцукенгшщзхъфывапролджэячсмитьбюё"
_EN2RU = dict(zip(_EN, _RU))
_RU2EN = dict(zip(_RU, _EN))


def L(text):
    """Тот же текст, набранный не в той раскладке: кириллица → латиница, если в тексте есть кириллица
    (латинские «3d» и цифры остаются как есть), иначе латиница → кириллица."""
    table = _RU2EN if any(ch in _RU2EN for ch in text) else _EN2RU
    return "".join(table.get(ch, ch) for ch in text)


def J(grade, **f):
    return (grade, f)


def FAM(fid, ren=2, other=None, other_grade=1):
    """Точное семейство: новые лицензии 3, продления ren; other — смежные семейства с оценкой other_grade."""
    rules = [J(3, fam=fid, not_lt=REN), J(ren, fam=fid, lt=REN, opt=True)]  # продлений может не быть
    if other:
        rules.append(J(other_grade, fam=other, not_lt=REN))
    return rules


def C(cid, j, q, **kw):
    """Кластер: id, разметка по умолчанию, запросы; kw: set, absent, terms, reform, note."""
    return dict(id=cid, j=j, q=q, **kw)


KASP_HOME = ["kaspersky-standard", "kaspersky-plus", "kaspersky-premium", "kaspersky-internet-security"]
KASP_ALL = KASP_HOME + ["kaspersky-small-office-security", "kaspersky-endpoint-security"]
DRWEB = ["drweb-security-space", "drweb-enterprise-security-suite"]
ESET = ["eset-nod32-antivirus", "eset-smart-security-premium", "eset-protect"]
AV_HOME = KASP_HOME + ["drweb-security-space", "eset-nod32-antivirus", "eset-smart-security-premium"]
AV_BIZ = ["kaspersky-small-office-security", "kaspersky-endpoint-security", "drweb-enterprise-security-suite",
          "eset-protect"]
AV_ALL = AV_HOME + AV_BIZ
RU_OFFICE = ["myoffice-standard", "r7-office-professional"]
MS_OFFICE = ["office-2021", "office-2024", "microsoft-365-home", "microsoft-365-business"]
CAD = ["autodesk-autocad", "autodesk-autocad-lt", "kompas-3d", "nanocad"]
KASP_NEW = J(3, fam=KASP_ALL, not_lt=REN)
KASP_REN = J(2, fam=KASP_ALL, lt=REN)

# ---------------------------------------------------------------- demo
DEMO = [
    C("demo-kaspersky-typo", [KASP_NEW, KASP_REN], [
        ("typo", "касперкий", 9, {"n": "F2: опечатка, исправление с плашкой «Искать как введено»"}),
        ("layout", "rfcgthcrbq", 4, {"n": "F2: раскладка; на проде уже исправляется — паритет"}),
    ], set="demo", reform="kaspersky-vendor"),
    C("demo-windows-slang", [J(3, fam="windows-11-pro", not_lt=["upgrade"]), J(2, ids=["w11p-upg"]),
                             J(1, fam=["windows-11-home", "windows-10-pro"])], [
        ("slang", "винда 11 про", 12, {"n": "F2: сленг из словаря — находит Windows 11 Pro без плашки"}),
    ], set="demo"),
    C("demo-photoshop-analog-20", [J(3, fam=["fotomaster", "affinity-photo", "coreldraw-graphics-suite"], unit=True),
                                   J(2, fam=["fotomaster", "affinity-photo", "coreldraw-graphics-suite"]),
                                   J(1, fam=["adobe-photoshop", "adobe-creative-cloud"])], [
        ("analog", "аналог фотошопа на 20 пользователей", 4,
         {"n": "F3: аналог + количество; 3 — аналоги, продаваемые поштучно; Photoshop сам не цель"}),
    ], set="demo"),
    C("demo-ru-office-30", [J(3, ids=["r7-p30", "r7-p1"]), J(2, fam=RU_OFFICE, lt=["perpetual"]),
                            J(1, fam=RU_OFFICE, lt=["subscription"])], [
        ("license", "отечественный офис на 30 компов бессрочка", 5,
         {"n": "F3: origin=ru строго, 30 устройств, бессрочно; МойОфис — на пользователя (уточняющий вопрос)"}),
    ], set="demo"),
    C("demo-kaspersky-10pc", [J(1, fam=KASP_ALL), J(3, fam=KASP_ALL, qty=10, not_lt=REN),
                              J(2, fam=KASP_ALL, qty=10, lt=REN)], [
        ("license", "касперский на 10 пк", 14, {"n": "F3: количество из запроса; прод его игнорирует"}),
    ], set="demo"),
    C("demo-solidworks", [J(2, ids=["kompas-perp", "kompas-rent1y"]), J(1, ids=["kompas-graf", "nano-mech"])], [
        ("absent", "solidworks", 7, {"n": "F4: товара нет, есть отечественный аналог КОМПАС-3D"}),
        ("absent", "солидворкс", 3, {"n": "F4: то же кириллицей"}),
    ], set="demo", absent=True, terms=["solidworks", "солидворкс"], reform="kompas"),
    C("demo-renew-antivirus", [J(3, fam=AV_ALL, lt=REN)], [
        ("renewal", "продлить антивирус", 3, {"c": "client-a",
                                              "n": "F5: клиент А покупал KSOS на 10 ПК — первым его продление",
                                              "j": [J(3, ids=["ksos-10pc-ren"]),
                                                    J(2, fam="kaspersky-small-office-security", lt=REN),
                                                    J(1, fam=AV_ALL, lt=REN)]}),
        ("renewal", "продлить антивирус", 15, {"c": "client-b",
                                               "n": "F5: без истории — подходит любое продление антивируса, "
                                                    "порядок по популярности"}),
        ("renewal", "продлить dr.web", 2, {"c": "client-a", "r": "renew-drweb",
                                           "n": "F5: явное в запросе важнее истории покупок",
                                           "j": [J(3, fam=DRWEB, lt=REN), J(1, fam=DRWEB, not_lt=REN)]}),
    ], set="demo", reform="renew-kaspersky"),
    C("demo-ws-16core", [J(3, ids=["ws22-std16", "ws25-std16", "ws22-dc16", "ws25-dc16"]),
                         J(1, ids=["ws22-std2", "ws25-std2"])], [
        ("license", "windows server на 16 ядер", 6, {"n": "F3: метрика «ядро» и количество"}),
    ], set="demo"),
    C("demo-1c-5users", [J(3, ids=["1ccl-5", "1ccl-5usb"]), J(2, ids=["1ccl-1"]), J(1, ids=["1ccl-10"]),
                         J(1, fam=["1c-accounting", "1c-trade-management"], ed="ПРОФ")], [
        ("license", "1с на 5 пользователей", 8,
         {"n": "F3: «пользователи» в 1С — клиентская лицензия на рабочие места"}),
    ], set="demo"),
]

# ---------------------------------------------------------------- точные названия и их формулировки
W11PRO = [J(3, fam="windows-11-pro", not_lt=["upgrade"]), J(2, ids=["w11p-upg"]),
          J(1, fam=["windows-11-home", "windows-10-pro"])]

EXACT = [
    C("kaspersky-vendor", [KASP_NEW, KASP_REN], [
        ("slang", "касперский", 85), ("exact", "kaspersky", 40), ("slang", "каспер", 11),
        ("typo", "каспеский", 6), ("layout", L("kaspersky"), 2),
    ], note="вендорный запрос: любая позиция Kaspersky"),
    C("kaspersky-plus", FAM("kaspersky-plus", other=["kaspersky-standard", "kaspersky-premium",
                                                     "kaspersky-internet-security"]), [
        ("exact", "kaspersky plus", 34), ("slang", "касперский плюс", 18, {"n": "транслит редакции"}),
        ("typo", "kaspersky plas", 3),
    ]),
    C("kaspersky-premium", FAM("kaspersky-premium", other=["kaspersky-standard", "kaspersky-plus"]), [
        ("exact", "kaspersky premium", 26), ("slang", "касперский премиум", 12, {"n": "транслит редакции"}),
    ]),
    C("kaspersky-standard", FAM("kaspersky-standard", other=["kaspersky-plus", "kaspersky-premium"]), [
        ("exact", "kaspersky standard", 15), ("slang", "касперский стандарт", 9, {"n": "транслит редакции"}),
    ]),
    C("kis-exact", FAM("kaspersky-internet-security", other="kaspersky-plus", other_grade=2), [
        ("exact", "kaspersky internet security", 22),
        ("slang", "касперский интернет секьюрити", 8, {"n": "транслит"}),
        ("typo", "kaspersky internet sequrity", 2),
    ], note="Plus — преемник KIS, подходящая альтернатива"),
    C("ksos-exact", FAM("kaspersky-small-office-security", other="kaspersky-endpoint-security"), [
        ("exact", "kaspersky small office security", 18),
            ]),
    C("kes-exact", FAM("kaspersky-endpoint-security", other="kaspersky-small-office-security"), [
        ("exact", "kaspersky endpoint security для бизнеса", 10),
        ("exact", "касперский endpoint security расширенный", 3,
         {"j": [J(3, fam="kaspersky-endpoint-security", ed="Расширенный"), J(1, fam="kaspersky-endpoint-security")]}),
    ]),
    C("drweb-vendor", [J(3, fam=DRWEB, not_lt=REN), J(2, fam=DRWEB, lt=REN)], [
        ("slang", "доктор веб", 16), ("exact", "dr.web", 12),
    ]),
    C("drweb-ss", FAM("drweb-security-space", other="drweb-enterprise-security-suite"), [
        ("exact", "dr.web security space", 14),
    ]),
    C("drweb-ess", FAM("drweb-enterprise-security-suite", other="drweb-security-space"), [
        ("exact", "dr.web enterprise security suite", 6),
        ("exact", "dr.web для бизнеса", 5, {"n": "в названии нет «для бизнеса» — нужна семантика"}),
    ]),
    C("eset-home", FAM("eset-nod32-antivirus", other="eset-smart-security-premium", other_grade=2), [
        ("exact", "eset nod32", 12),
        ("exact", "eset smart security premium", 6, {"j": FAM("eset-smart-security-premium",
                                                              other="eset-nod32-antivirus")}),
        ("slang", "есет", 5, {"j": [J(3, fam=ESET, not_lt=REN), J(2, fam=ESET, lt=REN)]}),
    ]),
    C("eset-protect", FAM("eset-protect", other=["eset-nod32-antivirus", "eset-smart-security-premium"]), [
        ("exact", "eset protect", 4),
    ]),
    C("windows-11", [J(3, fam=["windows-11-home", "windows-11-pro"], not_lt=["upgrade"]), J(2, ids=["w11p-upg"]),
                     J(1, fam="windows-10-pro")], [
        ("exact", "windows 11", 45), ("slang", "винда 11", 16),
        ("layout", "dbylf 11", 6, {"n": "«винда 11» в английской раскладке"}),
    ]),
    C("windows-11-pro", W11PRO, [
        ("exact", "windows 11 pro", 70), ("typo", "windiws 11 pro", 2), ("slang", "виндовс 11 про", 14),
        ("typo", "windows 11 prо", 3, {"n": "смешанный алфавит: кириллическая «о» в pro"}),
        ("layout", L("windows 11 pro"), 3),
        ("license", "windows 11 pro oem", 6, {"j": [J(3, ids=["w11p-oem"]), J(2, ids=["w11p-esd"]),
                                                   J(1, ids=["w11p-upg"])]}),
    ]),
    C("windows-11-home", [J(3, fam="windows-11-home"), J(1, fam="windows-11-pro")], [
        ("exact", "windows 11 home", 30),
        ("slang", "windows 11 домашняя", 9, {"n": "русское имя редакции, в карточке только Home"}),
        ("typo", "windows 11 hom", 2),
    ]),
    C("windows-10-pro", [J(3, fam="windows-10-pro"), J(2, ids=["w11p-esd", "w11p-oem"])], [
        ("exact", "windows 10 pro", 28),
        ("license", "windows 10 pro oem", 5, {"j": [J(3, ids=["w10p-oem"]), J(2, ids=["w10p-esd"])]}),
    ], note="Windows 11 Pro даёт право downgrade до 10 Pro — подходящая альтернатива"),
    C("ws2022", [J(3, fam="windows-server-2022"), J(1, fam="windows-server-cal")], [
        ("exact", "windows server 2022", 22),
        ("exact", "windows server 2022 standard", 12, {"j": [J(3, ids=["ws22-std16"]), J(2, ids=["ws22-std2"]),
                                                             J(1, ids=["ws22-dc16"])]}),
                    ], note="версия строгая: 2025 не подходит"),
    C("ws-cal", [J(3, fam="windows-server-cal"), J(1, fam="windows-server-2022")], [
        ("exact", "windows server cal", 8),
        ("license", "cal на пользователя windows server 2022", 4,
         {"j": [J(3, ids=["wscal-u1", "wscal-u5"]), J(1, ids=["wscal-d1", "wscal-d5", "wscal-rds"])]}),
        ("exact", "rds cal", 3, {"j": [J(3, ids=["wscal-rds"]), J(1, fam="windows-server-cal")]}),
    ]),
    C("ws2025", [J(3, fam="windows-server-2025")], [
        ("exact", "windows server 2025", 14),
        ("license", "windows server 2025 standard 16 core", 4,
         {"j": [J(3, ids=["ws25-std16"]), J(2, ids=["ws25-dc16"]), J(1, ids=["ws25-std2"])]}),
    ]),
    C("sql", [J(3, fam="sql-server-2022")], [
        ("exact", "microsoft sql server 2022", 9),
        ("license", "sql server на 4 ядра", 3, {"j": [J(3, ids=["sql22-std2c", "sql22-ent2c"]),
                                                     J(1, ids=["sql22-stdsrv"])]}),
    ]),
    C("office-2021", [J(3, fam="office-2021")], [
        ("exact", "office 2021", 26), ("slang", "офис 2021", 18),
        ("exact", "microsoft office 2021 для дома и бизнеса", 11,
         {"j": [J(3, ids=["off21-hb"]), J(1, fam="office-2021")]}),
        ("exact", "office 2021 профессиональный плюс", 6,
         {"j": [J(3, ids=["off21-pp"]), J(2, ids=["off21-ltsc"]), J(1, fam="office-2021")]}),
    ], note="версия строгая: 2024 не подходит"),
    C("office-2024", [J(3, fam="office-2024")], [
        ("exact", "office 2024", 22),
        ("slang", "офис 2024 для дома и бизнеса", 8, {"j": [J(3, ids=["off24-hb"]), J(1, fam="office-2024")]}),
        ("exact", "microsoft office 2024 home", 5, {"n": "в карточке «для дома», не Home",
                                                    "j": [J(3, ids=["off24-home"]), J(2, ids=["off24-hb"]),
                                                          J(1, fam="office-2024")]}),
            ]),
    C("office-generic", [J(3, fam=["office-2021", "office-2024"]), J(2, fam=["microsoft-365-home",
                                                                              "microsoft-365-business"])], [
        ("exact", "microsoft office", 38), ("layout", L("microsoft office"), 2),
        ("slang", "ворд", 14, {"n": "Word входит в пакеты Office и Microsoft 365"}),
        ("slang", "эксель", 9),
    ]),
    C("m365", [J(3, fam=["microsoft-365-home", "microsoft-365-business"])], [
        ("exact", "microsoft 365", 30), ("slang", "офис 365", 9),
        ("exact", "microsoft 365 business standard", 12,
         {"j": [J(3, ids=["m365b-bs"]), J(1, fam="microsoft-365-business")]}),
            ]),
    C("myoffice", FAM("myoffice-standard", other="r7-office-professional"), [
        ("exact", "мойофис", 14), ("slang", "мой офис стандартный", 9),
        ("slang", "myoffice", 4, {"n": "транслит, в карточке только кириллица"}),
    ]),
    C("r7", FAM("r7-office-professional", other="myoffice-standard"), [
        ("exact", "р7 офис", 12),             ]),
    C("astra", FAM("astra-linux-se"), [
        ("exact", "astra linux", 20), ("slang", "астра линукс", 12),
        ("layout", L("astra linux"), 3),
        ("exact", "astra linux special edition воронеж", 5,
         {"j": [J(3, ids=["astra-vrn", "astra-srv-vrn"]), J(1, fam="astra-linux-se")]}),
    ]),
    C("redos", FAM("red-os"), [
        ("exact", "ред ос", 9), ("slang", "red os", 4, {"n": "транслит"}),     ]),
    C("alt", FAM("alt-os"), [
        ("slang", "альт линукс", 6), ("slang", "alt linux", 3, {"n": "транслит"}),
        ("exact", "альт рабочая станция", 5, {"j": [J(3, ids=["alt-ws"]), J(1, fam="alt-os")]}),
    ]),
    C("1c-buh", [J(3, fam="1c-accounting"), J(1, fam="1c-client-license")], [
        ("exact", "1с бухгалтерия", 48), ("slang", "1c бухгалтерия", 15, {"n": "латинская c в «1c»"}),
        ("exact", "1с бухгалтерия проф", 22, {"j": [J(3, ids=["1cbuh-prof"]), J(2, ids=["1cbuh-cloud"]),
                                                    J(1, fam="1c-accounting")]}),
        ("exact", "1с бухгалтерия базовая", 9, {"j": [J(3, ids=["1cbuh-base"]), J(1, fam="1c-accounting")]}),
        ("typo", "1с бугалтерия", 4), ("typo", "1с бухгалтерея", 3), ("layout", L("1с бухгалтерия"), 2),
    ]),
    C("1c-ut", [J(3, fam="1c-trade-management"), J(1, fam="1c-complex-automation")], [
        ("exact", "1с управление торговлей", 18), ("slang", "1с ут", 8),
            ]),
    C("1c-client", [J(3, fam="1c-client-license")], [
        ("exact", "клиентская лицензия 1с", 12),
        ("license", "1с лицензия на 10 рабочих мест", 7,
         {"j": [J(3, ids=["1ccl-10"]), J(2, ids=["1ccl-1"]), J(1, fam="1c-client-license")]}),
        ("license", "1с:предприятие 8.3 клиентская лицензия на 20 рабочих мест", 4,
         {"j": [J(3, ids=["1ccl-20"]), J(2, ids=["1ccl-1"]), J(1, fam="1c-client-license")]}),
        ("license", "1с доп лицензия на 1 пользователя", 4,
         {"j": [J(3, ids=["1ccl-1"]), J(1, fam="1c-client-license")]}),
    ]),
    C("1c-server", [J(3, fam="1c-server")], [
        ("exact", "лицензия на сервер 1с", 8),
        ("exact", "сервер 1с 8.3 x64", 5, {"j": [J(3, ids=["1csrv-prof64", "1csrv-corp64"]), J(1, ids=["1csrv-prof32"])]}),
        ("exact", "1с сервер корп", 3, {"j": [J(3, ids=["1csrv-corp64"]), J(1, fam="1c-server")]}),
    ]),
    C("1c-its", [J(3, fam="1c-its")], [
        ("exact", "1с итс проф", 7, {"j": [J(3, ids=["1cits-prof12"]), J(1, ids=["1cits-tech12"])]}),
        ("renewal", "продление итс", 3),
    ]),
    C("1c-zup-exact", [J(3, fam="1c-zup")], [
        ("exact", "1с зарплата и управление персоналом", 14),
        ("exact", "1с зарплата и кадры", 6, {"n": "старое название продукта"}),
    ]),
    C("1c-roznica", [J(3, fam="1c-roznica")], [
        ("exact", "1с розница", 10), ("exact", "1с розница проф", 3, {"j": [J(3, ids=["1croz-prof"]),
                                                                          J(1, ids=["1croz-base"])]}),
    ]),
    C("1c-ka", [J(3, fam="1c-complex-automation")], [
        ("exact", "1с комплексная автоматизация", 7),
    ]),
    C("photoshop", FAM("adobe-photoshop", other="adobe-creative-cloud"), [
        ("exact", "adobe photoshop", 34), ("slang", "фотошоп", 40), ("typo", "фотшоп", 3), ("exact", "photoshop", 22),
        ("typo", "фтошоп", 6), ("layout", L("photoshop"), 3),
        ("license", "фотошоп для команды на год", 5,
         {"j": [J(3, ids=["ps-t1y"]), J(2, ids=["ps-i1y", "ps-tren"]), J(1, fam="adobe-creative-cloud")]}),
    ]),
    C("illustrator", FAM("adobe-illustrator", other="adobe-creative-cloud"), [
        ("exact", "adobe illustrator", 14), ("slang", "иллюстратор", 9),
    ]),
    C("creative-cloud", FAM("adobe-creative-cloud", other=["adobe-photoshop", "adobe-illustrator"]), [
        ("exact", "adobe creative cloud", 16),
        ("exact", "adobe cc all apps", 5, {"n": "сокращение CC, в названии полностью"}),
    ]),
    C("coreldraw", FAM("coreldraw-graphics-suite"), [
        ("exact", "coreldraw", 18), ("slang", "корел дро", 7),
        ("exact", "corel draw 2025", 4, {"n": "раздельное написание"}),
        ("license", "coreldraw бессрочная", 4, {"j": [J(3, fam="coreldraw-graphics-suite", lt=["perpetual"]),
                                                     J(1, fam="coreldraw-graphics-suite")]}),
    ]),
    C("fotomaster", FAM("fotomaster"), [
        ("exact", "фотомастер", 12), ("slang", "fotomaster", 3, {"n": "транслит"}),
        ("exact", "фотомастер премиум", 3, {"j": [J(3, ids=["fm-prem"]), J(1, fam="fotomaster")]}),
    ]),
    C("affinity", FAM("affinity-photo"), [
        ("exact", "affinity photo", 8),
    ]),
    C("autocad", FAM("autodesk-autocad", other="autodesk-autocad-lt"), [
        ("exact", "autocad", 36), ("slang", "автокад", 30), ("layout", L("autocad"), 2),
        ("license", "автокад на 1 год", 6, {"j": [J(3, ids=["acad-1y"]), J(2, ids=["acad-ren"]),
                                                 J(1, ids=["acad-3y", "acadlt-1y"])]}),
    ]),
    C("autocad-lt", FAM("autodesk-autocad-lt", other="autodesk-autocad"), [
        ("exact", "autocad lt", 12), ("slang", "автокад лт", 4, {"n": "«лт» не в словаре"}),
    ]),
    C("kompas", [J(3, ids=["kompas-perp", "kompas-rent1y"]), J(2, ids=["kompas-graf"]),
                 J(1, ids=["kompas-home"])], [
        ("exact", "компас 3d", 20), ("typo", "компасс 3d", 2), ("slang", "kompas 3d", 6, {"n": "транслит"}),
        ("layout", L("компас 3d"), 2),
        ("exact", "компас график", 4, {"j": [J(3, ids=["kompas-graf"]), J(1, fam="kompas-3d")]}),
    ], note="Home — только некоммерческое использование, для B2B частично"),
    C("nanocad", FAM("nanocad"), [
        ("exact", "nanocad", 14), ("slang", "нанокад", 9),
        ("exact", "nanocad плюс", 5, {"j": [J(3, fam="nanocad", ed="Плюс"), J(1, ids=["nano-mech"])]}),
    ]),
    C("renga", FAM("renga"), [
        ("exact", "renga", 6),     ]),
    C("vmware", FAM("vmware-vsphere"), [
        ("exact", "vmware vsphere", 12),
        ("exact", "vsphere foundation", 2, {"j": [J(3, ids=["vsph-vvf1y"]), J(1, fam="vmware-vsphere")]}),
    ]),
    C("veeam", FAM("veeam-backup"), [
        ("exact", "veeam backup", 14), ("slang", "виам", 3),
        ("typo", "veam backup", 3),
    ]),
    C("cyber-backup", FAM("cyber-backup"), [
        ("exact", "кибер бэкап", 9), ("typo", "кибер бекап", 3, {"n": "е вместо э"}),
    ]),
    C("postgres", FAM("postgres-pro"), [
        ("exact", "postgres pro", 6), ("slang", "постгрес про", 3),
        ("exact", "postgres pro enterprise", 2, {"j": [J(3, ids=["pgpro-ent"]), J(1, fam="postgres-pro")]}),
    ]),
    C("zoom", FAM("zoom-workplace"), [
        ("exact", "zoom", 18), ("slang", "зум", 9),
            ]),
    C("trueconf", FAM("trueconf-server"), [
        ("exact", "trueconf server", 9),
        ("license", "trueconf на 25 подключений", 3, {"j": [J(3, ids=["tc-p25"]), J(1, fam="trueconf-server")]}),
    ]),
    C("y360", FAM("yandex-360"), [
        ("exact", "яндекс 360", 16),
        ("slang", "yandex 360", 4, {"n": "транслит, в карточке «Яндекс»"}),
    ]),
    C("intellij", FAM("intellij-idea", other="jetbrains-all-products", other_grade=2), [
        ("exact", "intellij idea ultimate", 12),
        ("typo", "intelij idea", 3),
    ]),
    C("jetbrains", [J(3, fam=["intellij-idea", "jetbrains-all-products", "pycharm"], not_lt=REN),
                    J(2, fam=["intellij-idea", "jetbrains-all-products", "pycharm"], lt=REN)], [
        ("exact", "jetbrains", 8),
        ("exact", "jetbrains all products pack", 5, {"j": FAM("jetbrains-all-products",
                                                              other=["intellij-idea", "pycharm"])}),
    ]),
    C("pycharm", FAM("pycharm", other="jetbrains-all-products", other_grade=2), [
        ("exact", "pycharm professional", 7), ("slang", "пайчарм", 3),
    ]),
    C("visual-studio", FAM("visual-studio"), [
        ("exact", "visual studio professional", 8,
         {"j": [J(3, ids=["vs-pro22", "vs-prosub"]), J(1, ids=["vs-entsub"])]}),
            ]),
    C("teamviewer", FAM("teamviewer"), [
        ("exact", "teamviewer", 26), ("slang", "тимвьювер", 14), ("layout", L("teamviewer"), 2),
        ("typo", "teamveiwer", 3),
            ]),
    C("anydesk", FAM("anydesk"), [
        ("exact", "anydesk", 14),
        ("exact", "anydesk standard", 4, {"j": [J(3, ids=["ad-std1y"]), J(1, fam="anydesk")]}),
    ]),
    C("assistant", FAM("assistant"), [
        ("exact", "ассистент удаленный доступ", 6),
    ]),
    C("acrobat", FAM("adobe-acrobat"), [
        ("exact", "adobe acrobat pro", 18, {"j": [J(3, ids=["acro-pro1y"]), J(2, ids=["acro-proren"]),
                                                 J(1, ids=["acro-std1y"])]}),
        ("slang", "акробат", 9), ("typo", "adobe acrobt pro", 2),
            ]),
    C("finereader", FAM("abbyy-finereader-pdf", other="contentreader-pdf"), [
        ("exact", "abbyy finereader pdf", 14), ("slang", "файнридер", 6),
        ("typo", "abby finereader", 4),
    ]),
    C("contentreader", FAM("contentreader-pdf"), [
        ("exact", "contentreader pdf", 4),
    ]),
    C("secretnet", FAM("secret-net-studio"), [
        ("exact", "secret net studio", 6), ("slang", "секрет нет студио", 2),
    ]),
    C("infowatch", FAM("infowatch-traffic-monitor"), [
        ("exact", "infowatch traffic monitor", 4),     ]),
    C("maxpatrol", FAM("maxpatrol-vm"), [
        ("exact", "maxpatrol vm", 4),
    ]),
    C("ptaf", FAM("pt-application-firewall"), [
        ("exact", "pt application firewall", 3),
    ]),
]

# ---------------------------------------------------------------- артикулы (один кластер — одна позиция)
# "@sku:<product_id>" подставляется артикулом позиции при генерации.
SKU_ITEMS = [
    ("kis-2dev-1y", [("KL1939RDBFS", 3)]),
    ("w11p-esd", [("FQC-10572", 4), ("fqc10572", 1, "без дефиса, нижний регистр")]),
    ("w11p-oem", [("fqc-10528", 3)]),
    ("w11h-oem", [("KW9-00632", 3)]),
    ("w10p-oem", [("FQC-08909", 2)]),
    ("ws22-std16", [("P73-08328", 3)]),
    ("wscal-u1", [("R18-06466", 3)]),
    ("off21-hb", [("T5D-03511", 4)]),
    ("off24-hb", [("EP2-06684", 2)]),
    ("sql22-std2c", [("7NQ-01564", 1)]),
    ("acad-1y", [("057R1-WW8695-L562", 2)]),
    ("ps-t1y", [("65297615BA01A12", 2)]),
    ("acro-pro1y", [("65324058BA01A12", 1)]),
    ("vs-pro22", [("C5E-01391", 1)]),
    ("1cbuh-prof", [("@sku:1cbuh-prof", 3)]),
    ("kplus-3dev-1y", [("@sku:kplus-3dev-1y", 2)]),
]
