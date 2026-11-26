"""Юнит-тесты customer_eval: нормализация URL, первая цель ответа, счётчики, причина промаха.

  cd scripts/eval && python3 -m unittest
"""
import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import customer_eval as ce  # noqa: E402


class NormalizeURL(unittest.TestCase):
    def eq(self, a, b):
        self.assertEqual(ce.normalize_url(a), ce.normalize_url(b), f"{a!r} vs {b!r}")
        self.assertTrue(ce.same_url(a, b))

    def ne(self, a, b):
        self.assertNotEqual(ce.normalize_url(a), ce.normalize_url(b), f"{a!r} vs {b!r}")
        self.assertFalse(ce.same_url(a, b))

    def test_canonical_form(self):
        self.assertEqual(ce.normalize_url("/anydesk/anydesk/"), "https://store.softline.ru/anydesk/anydesk")
        self.assertEqual(ce.normalize_url("/cyberprotect/?filter[category][]=1033"),
                         "https://store.softline.ru/cyberprotect?filter[category][]=1033")

    def test_relative_equals_absolute(self):
        self.eq("/anydesk/anydesk/", "https://store.softline.ru/anydesk/anydesk/")
        self.eq("anydesk/anydesk/", "https://store.softline.ru/anydesk/anydesk/")
        self.eq("store.softline.ru/anydesk/anydesk/", "/anydesk/anydesk/")
        self.eq("//store.softline.ru/anydesk/anydesk/", "/anydesk/anydesk/")

    def test_scheme_and_www_of_store(self):
        self.eq("http://store.softline.ru/webinar/", "/webinar/")
        self.eq("https://www.store.softline.ru/webinar/", "/webinar/")
        self.eq("https://store.softline.ru:443/webinar/", "/webinar/")

    def test_host_case(self):
        self.eq("https://STORE.Softline.RU/webinar/", "/webinar/")
        self.eq("https://Example.COM/a", "https://example.com/a")

    def test_other_host_differs(self):
        self.ne("https://softline.ru/webinar/", "/webinar/")
        self.ne("https://example.com:8443/a", "https://example.com/a")

    def test_trailing_slash(self):
        self.eq("/webinar", "/webinar/")
        self.eq("/webinar//", "/webinar")
        self.eq("https://store.softline.ru", "https://store.softline.ru/")

    def test_percent_encoding_of_query(self):
        self.eq("/cyberprotect/?filter%5Bcategory%5D%5B%5D=1033", "/cyberprotect/?filter[category][]=1033")
        self.eq("/cyberprotect/?filter%5bcategory%5d%5b%5d=1033", "/cyberprotect/?filter[category][]=1033")

    def test_percent_encoding_of_path(self):
        self.eq("/kripto-pro/%D0%BA%D1%80%D0%B8%D0%BF%D1%82%D0%BE/", "/kripto-pro/крипто/")

    def test_indexed_and_empty_brackets(self):
        self.eq("/cyberprotect/?filter[category][0]=1033", "/cyberprotect/?filter[category][]=1033")
        self.eq("/cyberprotect/?filter%5Bcategory%5D%5B0%5D=1033", "/cyberprotect/?filter[category][]=1033")
        self.eq("/x/?filter[category][12]=1", "/x/?filter[category][]=1")

    def test_param_order(self):
        self.eq("/cyberprotect/?filter[category][]=1033&page=2", "/cyberprotect/?page=2&filter[category][0]=1033")

    def test_fragment_dropped(self):
        self.eq("/anydesk/anydesk/#reviews", "/anydesk/anydesk/")

    def test_different_values_differ(self):
        self.ne("/cyberprotect/?filter[category][]=1033", "/cyberprotect/?filter[category][]=1034")
        self.ne("/cyberprotect/?filter[category][]=1033", "/cyberprotect/")
        self.ne("/anydesk/anydesk/", "/anydesk/")

    def test_empty(self):
        self.assertEqual(ce.normalize_url(""), "")
        self.assertEqual(ce.normalize_url(None), "")
        self.assertEqual(ce.normalize_url("   "), "")
        self.assertFalse(ce.same_url("", ""))
        self.assertFalse(ce.same_url(None, "/webinar/"))

    def test_customer_targets_are_stable(self):
        # каждый эталон заказчика равен себе в абсолютной форме и не меняется от повторной нормализации
        with open(ce.QUERIES, encoding="utf-8") as f:
            lines = [line for line in f if line.strip()]
        self.assertEqual(len(lines), 10)
        for line in lines:
            u = json.loads(line)["target_url"]
            n = ce.normalize_url(u)
            self.assertEqual(ce.normalize_url(n), n)
            self.eq(u, "https://store.softline.ru" + u)


class FirstTarget(unittest.TestCase):
    def test_target_field_wins(self):
        resp = {"target": {"type": "vendor_category", "url": "https://store.softline.ru/cyberprotect/?filter[category][]=1033"},
                "items": [{"family_url": "https://store.softline.ru/cyberprotect/kiber-bekap/"}]}
        url, typ, src = ce.first_target(resp)
        self.assertEqual((typ, src), ("vendor_category", "target"))
        self.assertTrue(ce.same_url(url, "/cyberprotect/?filter[category][0]=1033"))

    def test_family_then_offer_then_vendor(self):
        self.assertEqual(ce.first_target({"items": [{"family_url": "/a/b/", "offer_url": "/a/b/1/"}]})[:2], ("/a/b/", "family"))
        self.assertEqual(ce.first_target({"items": [{"family_url": "", "offer_url": "/a/b/1/"}]})[:2], ("/a/b/1/", "sku"))
        self.assertEqual(ce.first_target({"items": [{"vendor_url": "/webinar/"}]})[:2], ("/webinar/", "vendor"))

    def test_no_url(self):
        self.assertEqual(ce.first_target({"items": [{"product_id": "x", "title": "t"}]})[0], "")
        self.assertEqual(ce.first_target({"items": []})[0], "")
        self.assertEqual(ce.first_target({})[0], "")


class Counts(unittest.TestCase):
    def test_families_and_skus(self):
        resp = {"items": [
            {"family_id": "f1", "product_id": "p1", "group": "exact", "variants": [{"product_id": "p2"}, {"product_id": "p1"}]},
            {"family_id": "f2", "offer_id": "o3", "match_type": "variant"},
            {"family_id": "f2", "offer_id": "o4", "match_type": "variant"},
        ]}
        c = ce.counts(resp)
        self.assertEqual((c["cards"], c["families"], c["skus"]), (3, 2, 4))
        self.assertEqual(c["match_type"], {"variant": 2})
        self.assertEqual(c["group"], {"exact": 1})

    def test_empty(self):
        c = ce.counts({})
        self.assertEqual((c["cards"], c["families"], c["skus"]), (0, 0, 0))


class MissReason(unittest.TestCase):
    resp = {"items": [{"family_id": "1"}, {"family_id": "391338"}]}

    def test_reasons(self):
        self.assertEqual(ce.miss_reason(True, {"group_id": "1"}, self.resp), "")
        self.assertEqual(ce.miss_reason(False, {"group_id": "1"}, self.resp), "ссылка")
        self.assertEqual(ce.miss_reason(False, {"group_id": "391338"}, self.resp), "ранг 2")
        self.assertEqual(ce.miss_reason(False, {"group_id": "777"}, self.resp), "цель")
        self.assertEqual(ce.miss_reason(False, {"vendor_slug": "webinar"}, self.resp), "цель")


class Percentile(unittest.TestCase):
    def test_nearest_rank(self):
        v = list(range(1, 18))  # 17 значений, как в прогоне
        self.assertEqual(ce.percentile(v, 50), 9)
        self.assertEqual(ce.percentile(v, 95), 17)
        self.assertIsNone(ce.percentile([], 50))


if __name__ == "__main__":
    unittest.main()
