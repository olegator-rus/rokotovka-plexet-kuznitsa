"""Кластеры запросов (часть 2): лицензии, аналоги, назначение, продления, absent, alias_gap, нетоварные, шум."""

from defs_queries import (C, J, FAM, REN, KASP_ALL, DRWEB, ESET, AV_HOME, AV_BIZ, AV_ALL, RU_OFFICE,
                          MS_OFFICE, CAD)

RU_OS = ["astra-linux-se", "red-os", "alt-os"]
ADOBE = ["adobe-photoshop", "adobe-illustrator", "adobe-creative-cloud", "adobe-acrobat"]
JB = ["intellij-idea", "jetbrains-all-products", "pycharm"]

# ---------------------------------------------------------------- условия лицензии
LICENSE = [
    C("kaspersky-5pc", [J(1, fam=KASP_ALL), J(3, fam=KASP_ALL, qty=5, not_lt=REN), J(2, fam=KASP_ALL, qty=5, lt=REN)], [
        ("license", "касперский на 5 пк", 9), ("license", "kaspersky на 5 устройств", 5),
    ]),
    C("kplus-3dev-2y", [J(3, ids=["kplus-3dev-2y"]), J(1, fam="kaspersky-plus")], [
        ("license", "kaspersky plus 3 устройства 2 года", 4),
        ("license", "касперский плюс на 3 устройства на 2 года", 3),
    ]),
    C("av-50", [J(3, fam=AV_BIZ, qty=50, not_lt=REN), J(2, fam=AV_BIZ, qty=50, lt=REN), J(1, fam=AV_BIZ, not_lt=REN)], [
        ("license", "антивирус для 50 компьютеров", 6),
        ("license", "kaspersky endpoint security на 50 узлов", 4,
         {"j": [J(3, fam="kaspersky-endpoint-security", qty=50, not_lt=REN),
                J(2, fam="kaspersky-endpoint-security", qty=50, lt=REN), J(1, fam="kaspersky-endpoint-security")]}),
    ]),
    C("office-perpetual", [J(3, fam=["office-2021", "office-2024"]), J(2, fam=RU_OFFICE, lt=["perpetual"])], [
        ("license", "офис бессрочно", 10), ("license", "office бессрочная лицензия", 5),
        ("license", "microsoft office бессрочно для дома и бизнеса", 3,
         {"j": [J(3, ids=["off21-hb", "off24-hb"]), J(1, fam=["office-2021", "office-2024"])]}),
    ]),
    C("ws-core-packs", [J(3, ids=["ws22-std16"]), J(2, ids=["ws22-dc16"]), J(1, ids=["ws22-std2"])], [
        ("license", "windows server 2022 standard на 16 ядер", 4),
        ("license", "лицензия windows server на 2 ядра", 3, {"j": [J(3, ids=["ws22-std2", "ws25-std2"])]}),
    ]),
    C("m365-10users", [J(3, fam="microsoft-365-business", unit=True), J(1, fam="microsoft-365-home")], [
        ("license", "microsoft 365 на 10 пользователей", 5,
         {"n": "подписки Microsoft 365 для бизнеса продаются поштучно на пользователя"}),
        ("license", "microsoft 365 business standard на год", 4,
         {"j": [J(3, ids=["m365b-bs"]), J(1, fam="microsoft-365-business")]}),
    ]),
    C("photoshop-5", [J(3, ids=["ps-t1y"]), J(2, ids=["ps-tren"]), J(1, ids=["ps-i1y", "cc-t1y"])], [
        ("license", "photoshop на 5 пользователей", 4, {"n": "для команд — поштучно; для частных лиц — нет"}),
    ]),
    C("autocad-3y", [J(3, ids=["acad-3y"]), J(1, ids=["acad-1y", "acadlt-3y"])], [
        ("license", "autocad на 3 года", 4), ("license", "автокад подписка 3 года", 2),
    ]),
    C("astra-10", [J(3, fam="astra-linux-se", metric="device", not_lt=REN), J(1, ids=["astra-srv-vrn"])], [
        ("license", "astra linux на 10 рабочих станций", 3),
    ]),
    C("kprem-10", [J(3, ids=["kprem-10dev-1y"]), J(1, fam="kaspersky-premium")], [
        ("license", "kaspersky premium на 10 устройств", 4),
    ]),
    C("veeam-10", [J(3, ids=["veeam-vul10-1y", "veeam-vul10-3y"]), J(1, ids=["veeam-ess5-1y"])], [
        ("license", "veeam на 10 серверов", 3),
    ]),
    C("nanocad-perp", [J(3, ids=["nano-plusperp"]), J(2, ids=["nano-mech"]), J(1, ids=["nano-plus1y"])], [
        ("license", "nanocad бессрочная лицензия", 4),
    ]),
    C("kompas-perp", [J(3, ids=["kompas-perp"]), J(1, ids=["kompas-graf", "kompas-rent1y"])], [
        ("license", "компас 3d бессрочная", 3),
    ]),
    C("tv-premium-1y", [J(3, ids=["tv-prem1y"]), J(1, fam="teamviewer")], [
        ("license", "teamviewer premium на год", 3),
    ]),
    C("zoom-20", [J(3, fam="zoom-workplace")], [("license", "zoom на 20 пользователей", 3)]),
    C("y360-50", [J(3, fam="yandex-360")], [("license", "яндекс 360 на 50 сотрудников", 3)]),
    C("av-3dev-2y", [J(3, ids=["kplus-3dev-2y", "drss-3pc-2y"]), J(1, fam=AV_HOME, qty=3, not_lt=REN)], [
        ("license", "антивирус на 3 устройства на 2 года", 3),
    ]),
    C("r7-10", [J(3, ids=["r7-p10", "r7-p1"]), J(1, fam="r7-office-professional")], [
        ("license", "р7 офис на 10 рабочих мест бессрочно", 3),
    ]),
    C("cyber-srv-perp", [J(3, ids=["cyb-srv"]), J(2, ids=["cyb-virt"]), J(1, ids=["cyb-srv1y"])], [
        ("license", "кибер бэкап для сервера бессрочная", 3),
    ]),
    C("drweb-10", [J(3, fam="drweb-enterprise-security-suite", qty=10, not_lt=REN), J(2, ids=["dress-cx10-ren"]),
                   J(1, fam="drweb-security-space", not_lt=REN)], [
        ("license", "dr.web на 10 пк", 5), ("license", "доктор веб на 10 компьютеров", 3),
    ]),
    C("w11pro-5", [J(3, ids=["w11p-esd"]), J(2, ids=["w11p-oem"]), J(1, ids=["w11p-upg"])], [
        ("license", "windows 11 pro 5 лицензий", 3),
    ]),
    C("drss-2pc", [J(3, ids=["drss-2pc-1y"]), J(1, fam="drweb-security-space")], [
        ("license", "dr.web security space на 2 пк", 4),
    ]),
    C("ksos-10pc", [J(3, ids=["ksos-10pc-1y", "ksos-10pc-2y"]), J(2, ids=["ksos-10pc-ren"]),
                    J(1, fam="kaspersky-small-office-security")], [
        ("license", "kaspersky small office security 10 пк", 4),
    ]),
]

# ---------------------------------------------------------------- аналоги
ANALOG = [
    C("photoshop-analog", [J(3, fam=["fotomaster", "affinity-photo"]), J(2, fam="coreldraw-graphics-suite"),
                           J(1, fam=["adobe-photoshop", "adobe-creative-cloud"], not_lt=REN)], [
        ("analog", "аналог фотошопа", 22), ("analog", "замена photoshop", 6),
        ("analog", "отечественный аналог фотошопа", 4, {"n": "origin=ru строго", "j": [J(3, fam="fotomaster")]}),
    ], reform="fotomaster"),
    C("msoffice-analog", [J(3, fam=RU_OFFICE), J(1, fam=MS_OFFICE, not_lt=REN)], [
        ("analog", "аналог microsoft office", 12),
        ("analog", "российский офис", 9, {"j": [J(3, fam=RU_OFFICE)]}),
        ("analog", "замена офиса для госкомпании", 4, {"n": "в реестре ПО строго", "j": [J(3, fam=RU_OFFICE, reestr=True)]}),
    ]),
    C("windows-analog", [J(3, fam=RU_OS, cat="os-desktop", not_lt=REN), J(2, fam=RU_OS, cat="os-server")], [
        ("analog", "замена windows", 8), ("purpose", "отечественная операционная система", 10),
        ("analog", "linux для госучреждения в реестре", 4,
         {"n": "сертифицированные редакции — 3",
          "j": [J(3, ids=["astra-vrn", "astra-smol", "redos-wscert"]), J(2, fam=RU_OS, cat="os-desktop", not_lt=REN)]}),
    ]),
    C("autocad-analog", [J(3, fam="nanocad"), J(3, ids=["kompas-perp", "kompas-rent1y", "kompas-graf"]),
                         J(1, fam=["autodesk-autocad", "autodesk-autocad-lt"], not_lt=REN)], [
        ("analog", "аналог автокада", 10), ("analog", "российский аналог autocad", 5),
        ("analog", "чем заменить автокад lt", 3,
         {"j": [J(3, fam="nanocad", ed="Плюс"), J(3, ids=["kompas-graf"]), J(2, ids=["kompas-perp", "nano-mech"]),
                J(1, fam="autodesk-autocad-lt")]}),
    ], reform="nanocad"),
    C("tv-analog", [J(3, fam=["assistant", "anydesk"]), J(1, fam="teamviewer")], [
        ("analog", "замена teamviewer", 9),
        ("analog", "аналог тимвьювер российский", 4, {"j": [J(3, fam="assistant")]}),
    ], reform="assistant"),
    C("zoom-analog", [J(3, fam="trueconf-server"), J(2, fam="yandex-360"), J(1, fam="zoom-workplace")], [
        ("analog", "аналог zoom", 7),
        ("analog", "отечественная видеосвязь вместо zoom", 3, {"j": [J(3, fam="trueconf-server"), J(2, fam="yandex-360")]}),
    ]),
    C("sql-analog", [J(3, fam="postgres-pro")], [
        ("analog", "аналог ms sql server", 4), ("analog", "отечественная субд", 5),
    ]),
    C("acrobat-analog", [J(3, fam=["abbyy-finereader-pdf", "contentreader-pdf"]), J(1, fam="adobe-acrobat", not_lt=REN)], [
        ("analog", "аналог adobe acrobat", 4),
    ]),
    C("vmware-analog", [J(3, ids=["alt-virt"])], [
        ("analog", "замена vmware", 5), ("analog", "отечественная виртуализация", 3),
    ]),
    C("veeam-analog", [J(3, fam="cyber-backup", not_lt=REN)], [("analog", "аналог veeam", 3)]),
    C("kaspersky-analog", [J(3, fam=DRWEB, not_lt=REN), J(2, fam=ESET, not_lt=REN)], [
        ("analog", "аналог касперского", 4),
    ]),
]

# ---------------------------------------------------------------- назначение
PURPOSE = [
    C("drawing", [J(3, fam=CAD, not_lt=REN, not_ids=["kompas-home"]), J(1, ids=["kompas-home"]), J(1, fam="renga")], [
        ("purpose", "программа для чертежей", 12), ("purpose", "программа для черчения", 6),
        ("purpose", "чертежи по гост", 3, {"n": "ЕСКД есть только в карточке КОМПАС",
                                          "j": [J(3, fam=["kompas-3d", "nanocad"], not_ids=["kompas-home"]),
                                                J(1, fam=["autodesk-autocad", "autodesk-autocad-lt"], not_lt=REN)]}),
    ], reform="autocad"),
    C("bim", [J(3, fam="renga"), J(1, fam="nanocad")], [("purpose", "проектирование зданий bim", 4)]),
    C("pdf-ocr", [J(3, fam=["abbyy-finereader-pdf", "contentreader-pdf"]), J(3, ids=["acro-pro1y"]),
                  J(2, ids=["acro-std1y", "acro-proren"])], [
        ("purpose", "чем открыть pdf и распознать текст", 6),
        ("purpose", "распознавание текста с картинки", 5,
         {"j": [J(3, fam=["abbyy-finereader-pdf", "contentreader-pdf"]), J(2, ids=["acro-pro1y"])]}),
        ("purpose", "редактировать pdf", 8,
         {"j": [J(3, fam=["adobe-acrobat", "abbyy-finereader-pdf", "contentreader-pdf"], not_lt=REN),
                J(2, ids=["acro-proren"])]}),
    ]),
    C("accounting", [J(3, fam="1c-accounting"), J(2, fam="1c-complex-automation")], [
        ("purpose", "программа для бухгалтерии", 9), ("purpose", "программа для бухучета", 4),
    ]),
    C("payroll", [J(3, fam="1c-zup"), J(2, fam="1c-complex-automation")], [
        ("purpose", "учет зарплаты и кадров", 5),
    ]),
    C("retail", [J(3, fam=["1c-roznica", "1c-trade-management"]), J(2, fam="1c-complex-automation")], [
        ("purpose", "программа для магазина учет товаров", 5),
        ("purpose", "складской учет", 4, {"j": [J(3, fam="1c-trade-management"),
                                                J(2, fam=["1c-complex-automation", "1c-roznica"])]}),
    ]),
    C("av-office", [J(3, fam=AV_BIZ, not_lt=REN), J(1, fam=AV_HOME, not_lt=REN)], [
        ("purpose", "антивирус для офиса", 10), ("purpose", "защита компьютеров компании от вирусов", 4),
    ]),
    C("av-home", [J(3, fam=AV_HOME, not_lt=REN)], [("purpose", "антивирус для дома", 8)]),
    C("videoconf", [J(3, fam=["zoom-workplace", "trueconf-server"]), J(2, fam="yandex-360")], [
        ("purpose", "видеоконференции для компании", 7),
        ("purpose", "сервер видеоконференций", 3, {"j": [J(3, fam="trueconf-server")]}),
    ]),
    C("remote", [J(3, fam=["teamviewer", "anydesk", "assistant"])], [
        ("purpose", "удаленный доступ к компьютеру", 11),
    ]),
    C("backup", [J(3, fam="veeam-backup"), J(3, fam="cyber-backup", metric="server", not_lt=REN), J(2, ids=["cyb-ws"])], [
        ("purpose", "резервное копирование серверов", 6),
    ]),
    C("ide-java", [J(3, fam="intellij-idea", not_lt=REN), J(2, fam="jetbrains-all-products", not_lt=REN)], [
        ("purpose", "среда разработки java", 5),
    ]),
    C("ide-python", [J(3, fam="pycharm", not_lt=REN), J(2, fam="jetbrains-all-products", not_lt=REN),
                     J(1, ids=["vs-pro22", "vs-prosub"])], [("purpose", "ide для python", 4)]),
    C("photo-editor", [J(3, fam=["adobe-photoshop", "fotomaster", "affinity-photo"], not_lt=REN),
                       J(2, fam="adobe-creative-cloud", not_lt=REN), J(1, fam="coreldraw-graphics-suite")], [
        ("purpose", "редактор фото", 8), ("purpose", "программа для обработки фотографий", 4),
    ]),
    C("vector-editor", [J(3, fam=["adobe-illustrator", "coreldraw-graphics-suite"]),
                        J(2, fam="adobe-creative-cloud", not_lt=REN)], [("purpose", "векторный редактор", 5)]),
    C("mail-domain", [J(3, fam="yandex-360"), J(2, fam="microsoft-365-business")], [
        ("purpose", "корпоративная почта", 5), ("purpose", "почта для домена", 4),
    ]),
    C("dlp", [J(3, fam="infowatch-traffic-monitor")], [("purpose", "защита от утечек информации", 4)]),
    C("vuln", [J(3, fam="maxpatrol-vm")], [("purpose", "сканер уязвимостей", 3)]),
    C("nsd", [J(3, fam="secret-net-studio", not_lt=REN), J(2, ids=["sns-sup"])], [
        ("purpose", "сзи от нсд", 3, {"n": "«сзи» есть в словаре, «нсд» — нет"}),
    ]),
    C("virtualization", [J(3, fam="vmware-vsphere"), J(3, ids=["alt-virt"])], [("purpose", "виртуализация серверов", 4)]),
    C("server-os", [J(3, fam=["windows-server-2022", "windows-server-2025"], not_ids=["ws22-std2", "ws25-std2"]),
                    J(3, ids=["astra-srv-vrn", "redos-srv", "alt-srv"]), J(1, ids=["ws22-std2", "ws25-std2"])], [
        ("purpose", "операционная система для сервера", 5),
    ]),
    C("office-suite", [J(3, fam=MS_OFFICE + RU_OFFICE, not_lt=REN)], [
        ("purpose", "офисный пакет", 8), ("purpose", "программа для создания презентаций", 3),
    ]),
    C("db-1c", [J(3, fam="postgres-pro"), J(3, fam="sql-server-2022", not_ids=["sql22-cal"]), J(1, fam="1c-server")], [
        ("purpose", "база данных для 1с", 3),
    ]),
    C("video-edit", [J(2, ids=["cc-t1y", "cc-i1y"])], [
        ("purpose", "программа для монтажа видео", 4,
         {"n": "Premiere Pro есть только в составе Creative Cloud — CONTENT_GAP"}),
    ]),
]

# ---------------------------------------------------------------- продления
RENEWAL = [
    C("renew-kaspersky", [J(3, fam=KASP_ALL, lt=REN), J(1, fam=KASP_ALL, not_lt=REN)], [
        ("renewal", "продление касперского", 12), ("renewal", "продлить kaspersky", 6),
        ("renewal", "продление kaspersky plus", 4,
         {"j": [J(3, ids=["kplus-3dev-ren"]), J(2, ids=["kis-2dev-ren"]), J(1, fam="kaspersky-plus", not_lt=REN)]}),
    ]),
    C("renew-drweb", [J(3, fam=DRWEB, lt=REN), J(1, fam=DRWEB, not_lt=REN)], [
        ("renewal", "продление dr.web", 6), ("renewal", "продлить доктор веб", 4),
    ]),
    C("renew-ksos-10", [J(3, ids=["ksos-10pc-ren"]), J(1, fam="kaspersky-small-office-security", lt=REN)], [
        ("renewal", "продление kaspersky small office security на 10 пк", 4),
    ]),
    C("renew-kes", [J(3, fam="kaspersky-endpoint-security", lt=REN), J(1, fam="kaspersky-endpoint-security", not_lt=REN)], [
        ("renewal", "продление kaspersky endpoint security", 3),
    ]),
    C("renew-autocad", [J(3, ids=["acad-ren"]), J(1, ids=["acad-1y", "acad-3y"])], [("renewal", "продление autocad", 3)]),
    C("renew-eset", [J(3, ids=["nod-1dev-ren"]), J(1, ids=["eprot-10dev-ren"])], [("renewal", "продление eset nod32", 3)]),
    C("renew-adobe", [J(3, fam=ADOBE, lt=REN), J(1, fam=ADOBE, not_lt=REN)], [("renewal", "продлить подписку adobe", 3)]),
    C("renew-jetbrains", [J(3, fam=JB, lt=REN), J(1, fam=JB, not_lt=REN)], [("renewal", "продление jetbrains", 3)]),
    C("renew-astra", [J(3, ids=["astra-upd"])], [("renewal", "продление подписки astra linux", 2)]),
    C("renew-antivirus-ctx", [J(1, fam=AV_ALL, lt=REN)], [
        ("renewal", "продлить антивирус", 2, {"c": "client-d", "n": "госклиент с Dr.Web ESS на 50 ПК",
                                              "j": [J(3, ids=["dress-cx50-ren"]),
                                                    J(2, fam="drweb-enterprise-security-suite", lt=REN),
                                                    J(1, fam=AV_ALL, lt=REN)]}),
        ("renewal", "продлить антивирус", 2, {"c": "client-e", "n": "частный клиент с Kaspersky Plus на 3 устройства",
                                              "j": [J(3, ids=["kplus-3dev-ren"]), J(1, fam=AV_ALL, lt=REN)]}),
        ("renewal", "продлить антивирус", 2, {"c": "client-c", "n": "крупный клиент с KES на 50 узлов",
                                              "j": [J(3, ids=["kes-std50-ren"]),
                                                    J(2, fam="kaspersky-endpoint-security", lt=REN),
                                                    J(1, fam=AV_ALL, lt=REN)]}),
    ]),
]

# ---------------------------------------------------------------- товара нет в каталоге
ABSENT = [
    C("abs-chatgpt", [], [
        ("absent", "chatgpt plus", 14), ("absent", "подписка chatgpt", 12), ("absent", "чат гпт для компании", 8),
    ], absent=True, terms=["chatgpt"], note="ASSORTMENT_GAP: аналога в каталоге нет"),
    C("abs-figma", [], [
        ("absent", "figma", 12), ("absent", "фигма", 10), ("absent", "figma professional на 5 пользователей", 6),
    ], absent=True, terms=["figma", "фигма"], note="ASSORTMENT_GAP: аналога в каталоге нет"),
    C("abs-notion", [], [
        ("absent", "notion", 9), ("absent", "ноушн", 6),
        ("absent", "аналог notion", 4, {"j": [J(1, fam="yandex-360")], "n": "частично: Документы и Диск"}),
    ], absent=True, terms=["notion", "ноушн"], note="ASSORTMENT_GAP"),
    C("abs-visio", [], [
        ("absent", "visio", 8), ("absent", "microsoft visio 2021", 7), ("absent", "визио", 5),
    ], absent=True, terms=["visio", "визио"], note="ASSORTMENT_GAP"),
    C("abs-winrar", [], [
        ("absent", "winrar", 11), ("absent", "винрар", 7), ("absent", "архиватор winrar лицензия", 4),
    ], absent=True, terms=["winrar", "винрар", "архиватор"], note="ASSORTMENT_GAP"),
    C("abs-revit", [J(2, fam="renga", not_lt=REN)], [
        ("absent", "revit", 6), ("absent", "ревит", 5), ("absent", "autodesk revit 2025", 3),
    ], absent=True, terms=["revit", "ревит"], reform="renga", note="SUBSTITUTE_AVAILABLE: Renga"),
    C("abs-1c-kz", [], [
        ("absent", "1с розница для казахстана", 3, {"j": [J(1, fam="1c-roznica")]}),
        ("absent", "1с бухгалтерия для казахстана", 3, {"j": [J(1, fam="1c-accounting")]}),
    ], absent=True, terms=["казахстан"], note="нет редакции для Казахстана; российская — частично"),
    C("abs-acronis", [J(2, fam="cyber-backup", not_lt=REN)], [
        ("absent", "acronis", 8), ("absent", "acronis backup", 5), ("absent", "акронис", 4),
    ], absent=True, terms=["acronis", "акронис"], reform="cyber-backup", note="SUBSTITUTE_AVAILABLE: Кибер Бэкап"),
]

# ---------------------------------------------------------------- сленг, которого нет в словаре (работа агента)
GAP = [
    C("gap-kis", FAM("kaspersky-internet-security", other="kaspersky-plus", other_grade=2), [
        ("alias_gap", "кис", 10),
        ("alias_gap", "кис на 2 устройства", 6, {"j": [J(3, ids=["kis-2dev-1y"]), J(2, ids=["kis-2dev-ren"]),
                                                       J(1, fam="kaspersky-internet-security")]}),
        ("alias_gap", "кис продление", 5, {"j": [J(3, ids=["kis-2dev-ren"]), J(2, ids=["kplus-3dev-ren"]),
                                                 J(1, fam="kaspersky-internet-security")]}),
        ("alias_gap", "кис 3 пк", 4, {"j": [J(3, ids=["kis-3dev-1y"]), J(1, fam="kaspersky-internet-security")]}),
    ], terms=["кис"], reform="kis-exact", note="«кис» = Kaspersky Internet Security"),
    C("gap-akad", FAM("autodesk-autocad", other="autodesk-autocad-lt"), [
        ("alias_gap", "акад", 9),
        ("alias_gap", "акад лицензия на год", 8, {"j": [J(3, ids=["acad-1y"]), J(2, ids=["acad-ren"]),
                                                        J(1, ids=["acad-3y", "acadlt-1y"])]}),
        ("alias_gap", "акад лт", 6, {"j": FAM("autodesk-autocad-lt", other="autodesk-autocad")}),
        ("alias_gap", "акад на 3 года", 5, {"j": [J(3, ids=["acad-3y"]), J(1, ids=["acad-1y", "acadlt-3y"])]}),
    ], terms=["акад"], reform="autocad", note="«акад» = AutoCAD"),
    C("gap-zup", [J(3, fam="1c-zup")], [
        ("alias_gap", "зуп", 10),
        ("alias_gap", "1с зуп проф", 8, {"j": [J(3, ids=["1czup-prof"]), J(1, fam="1c-zup")]}),
        ("alias_gap", "зуп базовая", 5, {"j": [J(3, ids=["1czup-base"]), J(1, fam="1c-zup")]}),
        ("alias_gap", "зуп корп", 3, {"j": [J(3, ids=["1czup-corp"]), J(1, fam="1c-zup")]}),
    ], terms=["зуп"], reform="1c-zup-exact", note="«ЗУП» = 1С:Зарплата и управление персоналом"),
    C("gap-nod32", FAM("eset-nod32-antivirus", other="eset-smart-security-premium", other_grade=2), [
        ("alias_gap", "нод32", 8), ("alias_gap", "нод 32 антивирус", 6),
        ("alias_gap", "нод32 продление", 4, {"j": [J(3, ids=["nod-1dev-ren"]), J(1, fam="eset-nod32-antivirus")]}),
    ], terms=["нод"], reform="eset-home", note="«нод32» = ESET NOD32 кириллицей"),
    C("gap-ksos", FAM("kaspersky-small-office-security"), [
        ("alias_gap", "ксос", 5),
        ("alias_gap", "ксос на 10 пк", 4, {"j": [J(3, ids=["ksos-10pc-1y", "ksos-10pc-2y"]), J(2, ids=["ksos-10pc-ren"]),
                                                 J(1, fam="kaspersky-small-office-security")]}),
        ("alias_gap", "ксос продление 5 пк", 3, {"j": [J(3, ids=["ksos-5pc-ren"]),
                                                       J(1, fam="kaspersky-small-office-security", lt=REN)]}),
    ], terms=["ксос"], reform="ksos-exact", note="«КСОС» = Kaspersky Small Office Security"),
    C("gap-mssql", [J(3, fam="sql-server-2022")], [
        ("alias_gap", "эскуэль сервер", 3), ("alias_gap", "скуль сервер 2022", 3),
        ("alias_gap", "mssql standard", 4, {"j": [J(3, fam="sql-server-2022", ed="Standard"), J(2, ids=["sql22-cal"]),
                                                  J(1, ids=["sql22-ent2c"])]}),
    ], terms=["эскуэль", "скуль", "mssql"], reform="sql", note="разговорные названия Microsoft SQL Server"),
    C("gap-tv", FAM("teamviewer"), [
        ("alias_gap", "тимвьюер", 7),
        ("alias_gap", "тимвьюер бизнес", 4, {"j": [J(3, ids=["tv-biz1y"]), J(1, fam="teamviewer")]}),
        ("alias_gap", "тимвьюер на год", 3),
    ], terms=["тимвьюер"], reform="teamviewer", note="написание без «в»; в словаре только «тимвьювер»"),
    C("gap-vmware", FAM("vmware-vsphere"), [
        ("alias_gap", "вмваре", 4), ("alias_gap", "вмваре 16 ядер", 3), ("alias_gap", "лицензия вмваре", 3),
    ], terms=["вмваре"], reform="vmware", note="VMware кириллицей"),
]

# ---------------------------------------------------------------- нетоварные запросы и шум
NON_PRODUCT = [
    C("np-invoice", [], [("non_product", "счет на оплату", 9), ("non_product", "выставить счет", 5),
                         ("non_product", "реквизиты", 3)]),
    C("np-docs", [], [("non_product", "закрывающие документы", 6), ("non_product", "акт сверки", 4),
                      ("non_product", "упд", 3)]),
    C("np-delivery", [], [("non_product", "доставка лицензии", 4), ("non_product", "как получить ключ после оплаты", 3),
                          ("non_product", "где мой заказ", 3)]),
    C("np-contacts", [], [("non_product", "контакты менеджера", 4)]),
]

NOISE = [
    C("noise-keyboard", [], [("noise", "asdf", 4), ("noise", "фывапролд", 3), ("noise", "qwerty", 3), ("noise", "ааааа", 2)]),
    C("noise-digits", [], [("noise", "123", 5), ("noise", "1", 2), ("noise", "?", 1)]),
    C("noise-test", [], [("noise", "test", 4), ("noise", "hello", 2),
                         ("noise", "ghbdtn", 2, {"n": "«привет» в английской раскладке"})]),
]
