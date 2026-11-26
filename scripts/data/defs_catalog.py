"""Справочники mock-каталога: вендоры, категории, семейства продуктов (часть 1).

Позиция (SKU-уровень) задаётся вариантом V внутри семейства F. Id позиции = f"{prefix}-{code}".
Цены — ориентир розницы B2B-магазина ПО в рублях на ноябрь 2026; None — «цена по запросу».
"""

VENDORS = [
    ("microsoft", "Microsoft", "foreign"),
    ("astra", "Группа Астра", "ru"),
    ("redsoft", "РЕД СОФТ", "ru"),
    ("basealt", "Базальт СПО", "ru"),
    ("myoffice", "МойОфис", "ru"),
    ("r7", "Р7", "ru"),
    ("adobe", "Adobe", "foreign"),
    ("abbyy", "ABBYY", "foreign"),
    ("content-ai", "Content AI", "ru"),
    ("kaspersky", "Kaspersky", "ru"),
    ("drweb", "Доктор Веб", "ru"),
    ("eset", "ESET", "foreign"),
    ("ptsecurity", "Positive Technologies", "ru"),
    ("infowatch", "InfoWatch", "ru"),
    ("securitycode", "Код Безопасности", "ru"),
    ("1c", "1С", "ru"),
    ("corel", "Corel", "foreign"),
    ("serif", "Serif", "foreign"),
    ("ams-software", "AMS Software", "ru"),
    ("autodesk", "Autodesk", "foreign"),
    ("ascon", "АСКОН", "ru"),
    ("nanosoft", "Нанософт", "ru"),
    ("renga-software", "Renga Software", "ru"),
    ("vmware", "VMware", "foreign"),
    ("veeam", "Veeam", "foreign"),
    ("cyberprotect", "Киберпротект", "ru"),
    ("postgrespro", "Postgres Professional", "ru"),
    ("zoom", "Zoom", "foreign"),
    ("trueconf", "TrueConf", "ru"),
    ("yandex", "Яндекс", "ru"),
    ("jetbrains", "JetBrains", "foreign"),
    ("teamviewer", "TeamViewer", "foreign"),
    ("anydesk", "AnyDesk", "foreign"),
    ("safib", "САФИБ", "ru"),
]

CATEGORIES = [
    ("os", "Операционные системы", None),
    ("os-desktop", "ОС для рабочих станций", "os"),
    ("os-server", "Серверные ОС и лицензии доступа", "os"),
    ("office", "Офисное ПО", None),
    ("office-suite", "Офисные пакеты", "office"),
    ("office-pdf", "PDF и распознавание текста", "office"),
    ("security", "Антивирусы и информационная безопасность", None),
    ("antivirus-home", "Антивирусы для дома", "security"),
    ("antivirus-business", "Антивирусная защита для бизнеса", "security"),
    ("infosec", "Средства защиты информации", "security"),
    ("business-apps", "1С и учётные системы", None),
    ("1c-configs", "Конфигурации 1С", "business-apps"),
    ("1c-licenses", "Платформа, лицензии и сопровождение 1С", "business-apps"),
    ("graphics", "Графика и дизайн", None),
    ("graphics-photo", "Фото и растровая графика", "graphics"),
    ("graphics-vector", "Векторная графика", "graphics"),
    ("cad", "САПР и проектирование", None),
    ("cad-design", "САПР 2D/3D", "cad"),
    ("cad-bim", "BIM-проектирование", "cad"),
    ("infra", "Инфраструктура", None),
    ("virtualization", "Виртуализация", "infra"),
    ("backup", "Резервное копирование", "infra"),
    ("dbms", "Системы управления базами данных", "infra"),
    ("communication", "Коммуникации", None),
    ("videoconf", "Видеоконференции", "communication"),
    ("collaboration", "Почта и совместная работа", "communication"),
    ("dev", "Разработка", None),
    ("ide", "Среды разработки", "dev"),
    ("remote", "Удалённый доступ", None),
    ("remote-access", "Удалённое администрирование", "remote"),
]

# Формы единиц: (1, 2–4, 5+)
U_DEVICE = ("устройство", "устройства", "устройств")
U_PC = ("ПК", "ПК", "ПК")
U_USER = ("пользователь", "пользователя", "пользователей")
U_SERVER = ("сервер", "сервера", "серверов")
U_CORE = ("ядро", "ядра", "ядер")
U_SEAT = ("рабочее место", "рабочих места", "рабочих мест")
U_WS = ("рабочая станция", "рабочие станции", "рабочих станций")
U_NODE = ("узел", "узла", "узлов")
U_CONN = ("одновременное подключение", "одновременных подключения", "одновременных подключений")
U_INST = ("экземпляр", "экземпляра", "экземпляров")


def V(code, lt, metric, qty, months, ed, price, **o):
    """Вариант лицензии. o: sku, t (название целиком), u (поштучно), cat, a (атрибуты), d (доп. описание)."""
    return dict(code=code, lt=lt, metric=metric, qty=qty, months=months, ed=ed, price=price, o=o)


def F(fid, prefix, name, vendor, cat, desc, variants, attrs=None, unit=None, tn=None, ed=None, reestr=None):
    """Семейство продукта. unit: {metric: формы}; tn: имя в названии позиции; ed: редакция по умолчанию."""
    return dict(id=fid, prefix=prefix, name=name, vendor=vendor, cat=cat, desc=desc, variants=variants,
                attrs=attrs or {}, unit=unit or {}, tn=tn or name, ed=ed, reestr=reestr)


ESD = {"delivery": "электронная поставка (ESD)"}
OEM = {"delivery": "OEM, только с новым компьютером"}
OFFICE_APPS = {"apps": "Word, Excel, PowerPoint, Outlook"}

FAMILIES_1 = [
    # --- ОС для рабочих станций и серверов ---
    F("windows-11-home", "w11h", "Microsoft Windows 11 Home", "microsoft", "os-desktop",
      "Операционная система для домашних и небольших офисных компьютеров. Включает Microsoft Defender, "
      "BitLocker недоступен. Лицензия привязывается к одному компьютеру.",
      [V("esd", "perpetual", "device", 1, None, "", 14990, sku="KW9-00664", a=ESD),
       V("oem", "oem", "device", 1, None, "", 12490, sku="KW9-00632", a=OEM)],
      attrs={"platform": "x64", "language": "русский"}, ed="Home"),
    F("windows-11-pro", "w11p", "Microsoft Windows 11 Pro", "microsoft", "os-desktop",
      "Операционная система для бизнеса: присоединение к домену, BitLocker, групповые политики и Hyper-V. "
      "Подходит для рабочих мест в организациях любого размера.",
      [V("esd", "perpetual", "device", 1, None, "", 23990, sku="FQC-10572", a=ESD, u=True),
       V("oem", "oem", "device", 1, None, "", 19990, sku="FQC-10528", a=OEM),
       V("upg", "upgrade", "device", 1, None, "", 11490, sku="4YR-00316",
         t="Microsoft Windows 11 Pro, обновление с Home до Pro, 1 устройство",
         d="Переводит уже активированную Windows 11 Home на редакцию Pro без переустановки.")],
      attrs={"platform": "x64", "language": "русский"}, ed="Pro"),
    F("windows-10-pro", "w10p", "Microsoft Windows 10 Pro", "microsoft", "os-desktop",
      "Предыдущая версия Windows для бизнеса. Нужна для совместимости со старым оборудованием и "
      "отраслевым ПО; поддерживает присоединение к домену и BitLocker.",
      [V("esd", "perpetual", "device", 1, None, "", 21990, sku="FQC-09131", a=ESD, u=True),
       V("oem", "oem", "device", 1, None, "", 18490, sku="FQC-08909", a=OEM)],
      attrs={"platform": "x64", "language": "русский"}, ed="Pro"),
    F("windows-server-2022", "ws22", "Microsoft Windows Server 2022", "microsoft", "os-server",
      "Серверная операционная система Microsoft. Лицензируется по физическим ядрам: минимум 16 ядер на "
      "сервер, дополнительные ядра докупаются пакетами по 2. Для подключения пользователей нужны CAL.",
      [V("std16", "perpetual", "core", 16, None, "Standard", 109990, sku="P73-08328"),
       V("std2", "perpetual", "core", 2, None, "Standard", 14490, sku="9EM-00653", u=True,
         t="Microsoft Windows Server 2022 Standard, дополнительная лицензия на 2 ядра"),
       V("dc16", "perpetual", "core", 16, None, "Datacenter", None, sku="P71-09389")],
      unit={"core": U_CORE}),
    F("windows-server-cal", "wscal", "Microsoft Windows Server 2022 CAL", "microsoft", "os-server",
      "Клиентские лицензии доступа к Windows Server 2022: на пользователя или на устройство. "
      "Покупаются в дополнение к серверной лицензии.",
      [V("u1", "perpetual", "user", 1, None, "User CAL", 5290, sku="R18-06466", u=True,
         t="Microsoft Windows Server 2022 CAL, клиентская лицензия на 1 пользователя"),
       V("d1", "perpetual", "device", 1, None, "Device CAL", 4690, sku="R18-06448", u=True,
         t="Microsoft Windows Server 2022 CAL, клиентская лицензия на 1 устройство"),
       V("u5", "perpetual", "user", 5, None, "User CAL", 24990, sku="R18-06475",
         t="Microsoft Windows Server 2022 CAL, 5 клиентских лицензий на пользователя"),
       V("d5", "perpetual", "device", 5, None, "Device CAL", 21990, sku="R18-06457",
         t="Microsoft Windows Server 2022 CAL, 5 клиентских лицензий на устройство"),
       V("rds", "perpetual", "user", 1, None, "RDS CAL", 12990, sku="6VC-04499", u=True,
         t="Microsoft Remote Desktop Services 2022 CAL, клиентская лицензия на 1 пользователя",
         d="Нужна для работы пользователей с сервером терминалов.")],
      ed="CAL"),
    F("windows-server-2025", "ws25", "Microsoft Windows Server 2025", "microsoft", "os-server",
      "Актуальная версия серверной ОС Microsoft с горячим обновлением и улучшенным Active Directory. "
      "Лицензируется по ядрам: базовый пакет на 16 ядер и дополнительные пакеты по 2 ядра.",
      [V("std16", "perpetual", "core", 16, None, "Standard", 119990, sku="P73-08674"),
       V("std2", "perpetual", "core", 2, None, "Standard", 15790, sku="9EM-00745", u=True,
         t="Microsoft Windows Server 2025 Standard, дополнительная лицензия на 2 ядра"),
       V("dc16", "perpetual", "core", 16, None, "Datacenter", None, sku="P71-09676")],
      unit={"core": U_CORE}),
    F("sql-server-2022", "sql22", "Microsoft SQL Server 2022", "microsoft", "dbms",
      "Реляционная СУБД Microsoft для учётных и аналитических систем. Доступны лицензирование по ядрам "
      "и модель «сервер + клиентские лицензии».",
      [V("std2c", "perpetual", "core", 2, None, "Standard", 389990, sku="7NQ-01564", u=True,
         t="Microsoft SQL Server 2022 Standard, лицензия на 2 ядра"),
       V("stdsrv", "perpetual", "server", 1, None, "Standard", 109990, sku="228-11477",
         t="Microsoft SQL Server 2022 Standard, лицензия на сервер (модель Server + CAL)"),
       V("cal", "perpetual", "user", 1, None, "CAL", 24490, sku="359-06866", u=True,
         t="Microsoft SQL Server 2022 CAL, клиентская лицензия на 1 пользователя"),
       V("ent2c", "perpetual", "core", 2, None, "Enterprise", None, sku="7JQ-01564", u=True,
         t="Microsoft SQL Server 2022 Enterprise, лицензия на 2 ядра")]),
    F("astra-linux-se", "astra", "Astra Linux Special Edition 1.8", "astra", "os-desktop",
      "Российская операционная система на базе Linux из реестра отечественного ПО. Три уровня "
      "защищённости; «Воронеж» и «Смоленск» сертифицированы ФСТЭК для работы с конфиденциальной информацией.",
      [V("orel", "perpetual", "device", 1, None, "«Орёл» (базовый уровень)", 9900, u=True),
       V("vrn", "perpetual", "device", 1, None, "«Воронеж» (усиленный уровень)", 19900, u=True,
         a={"certificate": "ФСТЭК"}),
       V("smol", "perpetual", "device", 1, None, "«Смоленск» (максимальный уровень)", 25900, u=True,
         a={"certificate": "ФСТЭК, Минобороны"}),
       V("srv-vrn", "perpetual", "server", 1, None, "«Воронеж», серверная", 49900, cat="os-server",
         a={"certificate": "ФСТЭК"}),
       V("upd", "renewal", "device", 1, 12, "подписка на обновления", 4900,
         t="Astra Linux Special Edition 1.8, подписка на обновления для рабочей станции, продление на 1 год")],
      unit={"device": U_WS}, attrs={"platform": "x86-64"}),
    F("red-os", "redos", "РЕД ОС 8", "redsoft", "os-desktop",
      "Российская операционная система на базе Linux для рабочих станций и серверов. Входит в реестр "
      "отечественного ПО; сертифицированная редакция соответствует требованиям ФСТЭК.",
      [V("ws", "perpetual", "device", 1, None, "Рабочая станция, стандартная редакция", 8900, u=True),
       V("wscert", "perpetual", "device", 1, None, "Рабочая станция, сертифицированная редакция", 14900,
         u=True, a={"certificate": "ФСТЭК"}),
       V("srv", "perpetual", "server", 1, None, "Сервер, стандартная редакция", 32900, cat="os-server"),
       V("sup", "renewal", "device", 1, 12, "техническая поддержка", 3900,
         t="РЕД ОС 8, техническая поддержка для рабочей станции, продление на 1 год")],
      unit={"device": U_WS}, attrs={"platform": "x86-64"}),
    F("alt-os", "alt", "ОС Альт 10", "basealt", "os-desktop",
      "Семейство российских операционных систем на базе Linux от «Базальт СПО». Входит в реестр "
      "отечественного ПО, подходит для госучреждений и школ.",
      [V("ws", "perpetual", "device", 1, None, "Рабочая станция", 7400, u=True,
         t="Альт Рабочая станция 10, 1 рабочее место, бессрочная лицензия"),
       V("srv", "perpetual", "server", 1, None, "Сервер", 27900, cat="os-server",
         t="Альт Сервер 10, 1 сервер, бессрочная лицензия"),
       V("virt", "perpetual", "server", 1, None, "Виртуализация", 39900, cat="virtualization",
         t="Альт Виртуализация 10, 1 сервер, бессрочная лицензия",
         d="Платформа виртуализации серверов на базе KVM с веб-интерфейсом управления кластером.")],
      attrs={"platform": "x86-64"}),

    # --- Офисные пакеты ---
    F("microsoft-365-home", "m365h", "Microsoft 365 для дома", "microsoft", "office-suite",
      "Подписка на приложения Office и облачное хранилище OneDrive на 1 ТБ для каждого пользователя. "
      "Для домашнего использования.",
      [V("pers", "subscription", "user", 1, 12, "Персональный", 6990),
       V("fam", "subscription", "user", 6, 12, "Для семьи", 8990,
         t="Microsoft 365 Для семьи, до 6 пользователей, 1 год")],
      attrs=OFFICE_APPS, tn="Microsoft 365"),
    F("microsoft-365-business", "m365b", "Microsoft 365 для бизнеса", "microsoft", "office-suite",
      "Облачная подписка для организаций: почта Exchange, Teams, OneDrive и SharePoint. "
      "Business Standard и Premium включают настольные приложения Office.",
      [V("bb", "subscription", "user", 1, 12, "Business Basic", 7290, u=True,
         d="Только веб- и мобильные версии приложений."),
       V("bs", "subscription", "user", 1, 12, "Business Standard", 15190, u=True),
       V("bp", "subscription", "user", 1, 12, "Business Premium", 27590, u=True,
         d="Дополнительно включает Intune и Defender для бизнеса."),
       V("e3", "subscription", "user", 1, 12, "E3", None, u=True)],
      attrs={"apps": "Word, Excel, PowerPoint, Outlook, Teams"}, tn="Microsoft 365"),
    F("office-2021", "off21", "Microsoft Office 2021", "microsoft", "office-suite",
      "Бессрочная версия офисного пакета Microsoft для одного компьютера. Обновления безопасности без "
      "новых функций.",
      [V("hs", "perpetual", "device", 1, None, "для дома и учёбы", 12990, sku="79G-05339",
         a={"apps": "Word, Excel, PowerPoint"}),
       V("hb", "perpetual", "device", 1, None, "для дома и бизнеса", 26990, sku="T5D-03511", a=OFFICE_APPS),
       V("pp", "perpetual", "device", 1, None, "профессиональный плюс", 39990, sku="269-17186",
         a={"apps": "Word, Excel, PowerPoint, Outlook, Access, Publisher"}),
       V("ltsc", "perpetual", "device", 1, None, "LTSC Standard", None, u=True, a=OFFICE_APPS,
         d="Корпоративная версия по программе корпоративного лицензирования.")],
      unit={"device": U_PC}),
    F("office-2024", "off24", "Microsoft Office 2024", "microsoft", "office-suite",
      "Актуальная бессрочная версия офисного пакета Microsoft. Лицензия на один компьютер, "
      "без подписки и облачных функций Microsoft 365.",
      [V("home", "perpetual", "device", 1, None, "для дома", 13990, sku="EP2-06606",
         a={"apps": "Word, Excel, PowerPoint, OneNote"}),
       V("hb", "perpetual", "device", 1, None, "для дома и бизнеса", 28490, sku="EP2-06684", a=OFFICE_APPS),
       V("ltscpp", "perpetual", "device", 1, None, "LTSC профессиональный плюс", None, u=True,
         a={"apps": "Word, Excel, PowerPoint, Outlook, Access"}),
       V("ltscstd", "perpetual", "device", 1, None, "LTSC Standard", None, u=True, a=OFFICE_APPS)],
      unit={"device": U_PC}),
    F("myoffice-standard", "myo", "МойОфис Стандартный", "myoffice", "office-suite",
      "Российский офисный пакет для работы с документами, таблицами и презентациями в форматах DOCX, XLSX, "
      "PPTX и ODF. Входит в реестр отечественного ПО, работает на Windows, Linux и macOS.",
      [V("p1", "perpetual", "user", 1, None, "", 9900, u=True),
       V("p10", "perpetual", "user", 10, None, "", 94000),
       V("p25", "perpetual", "user", 25, None, "", 227500),
       V("p50", "perpetual", "user", 50, None, "", 440000),
       V("s1", "subscription", "user", 1, 12, "", 3300, u=True)],
      attrs={"platform": "Windows, Linux, macOS", "apps": "Текст, Таблица, Презентация"}),
    F("r7-office-professional", "r7", "Р7-Офис Профессиональный", "r7", "office-suite",
      "Российский офисный пакет: текстовый редактор, таблицы и презентации с совместимостью с форматами "
      "Microsoft Office. Входит в реестр отечественного ПО.",
      [V("p1", "perpetual", "device", 1, None, "десктопная версия", 8490, u=True),
       V("p10", "perpetual", "device", 10, None, "десктопная версия", 79900),
       V("p30", "perpetual", "device", 30, None, "десктопная версия", 228000),
       V("p50", "perpetual", "device", 50, None, "десктопная версия", 359000),
       V("cloud", "subscription", "user", 1, 12, "облачная версия", 3990, u=True)],
      unit={"device": U_SEAT}, attrs={"platform": "Windows, Linux, macOS"}),

    # --- PDF и распознавание ---
    F("adobe-acrobat", "acro", "Adobe Acrobat", "adobe", "office-pdf",
      "Создание, редактирование, подписание и защита PDF-документов. Версия Pro дополнительно распознаёт "
      "отсканированные документы и сравнивает файлы.",
      [V("pro1y", "subscription", "user", 1, 12, "Pro для команд", 29990, u=True, sku="65324058BA01A12"),
       V("std1y", "subscription", "user", 1, 12, "Standard для команд", 19990, u=True, sku="65324066BA01A12"),
       V("proren", "renewal", "user", 1, 12, "Pro для команд", 29990, u=True, sku="65324058BA02A12")]),
    F("abbyy-finereader-pdf", "fr", "ABBYY FineReader PDF 16", "abbyy", "office-pdf",
      "Программа для распознавания текста (OCR) и работы с PDF: превращает сканы и фотографии документов "
      "в редактируемые файлы Word и Excel, редактирует и сравнивает PDF.",
      [V("std", "perpetual", "device", 1, None, "Standard", 12990, u=True),
       V("biz", "perpetual", "device", 1, None, "Business", 22990, u=True),
       V("stdsub", "subscription", "user", 1, 12, "Standard", 6490, u=True),
       V("biz3y", "subscription", "user", 1, 36, "Business", 19990, u=True)],
      unit={"device": U_SEAT}),
    F("contentreader-pdf", "cr", "ContentReader PDF", "content-ai", "office-pdf",
      "Российская программа для распознавания текста и редактирования PDF: OCR сканов и фотографий, "
      "конвертация в редактируемые форматы, сравнение документов. Входит в реестр отечественного ПО.",
      [V("std", "perpetual", "device", 1, None, "Standard", 9990, u=True),
       V("biz", "perpetual", "device", 1, None, "Business", 18990, u=True),
       V("corp", "subscription", "user", 1, 12, "Corporate", 6990, u=True)],
      unit={"device": U_SEAT}),
]
