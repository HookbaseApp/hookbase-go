package hookbase

import "unicode"

// Character folding for destination slugs. Generated data — see the recipe below; do not hand-edit.
//
// deriveDestinationSlug (destinations.go) has to derive the same slug from the same name as every
// other Hookbase SDK, and the reference implementation — deriveDestinationSlug in
// sdk/node-sdk/src/resources/wire.ts — folds characters with String.prototype.normalize('NFKD')
// before dropping the non-spacing marks the decomposition leaves behind:
//
//	name.normalize('NFKD').replace(/\p{Mn}/gu, '').toLowerCase()
//
// Go's standard library has no NFKD — golang.org/x/text/unicode/norm has it, but this SDK is
// documented as stdlib-only — so the mapping is this table. Every character whose folded spelling
// contains ASCII alphanumerics is listed, plus the two whose folded spelling is empty: accented
// Latin including Vietnamese, ligatures, fullwidth forms, super- and subscripts, Roman numerals,
// fractions, circled and parenthesised letters and digits, CJK compatibility squares and the
// mathematical alphanumerics. Everything else is left alone, becomes a separator, and so matches the
// reference for free.
//
// Characters in a fold value that are not [a-z0-9] are written as a single hyphen, because the next
// step collapses every run of non-[a-z0-9] to one hyphen anyway: U+2155, one fifth, folds to "1-5"
// here where NFKD gives "1", U+2044, "5".
//
// The table is the same whether the mark step drops the U+0300-U+036F block or the whole Mn
// category — regenerated under both, it came out byte for byte identical, because a character whose
// NFKD ends in ASCII only ever leaves marks from that block behind. Widening the strip therefore
// changes deriveDestinationSlug and not this file.
//
// Regenerate with the reference engine itself, so the two cannot drift — node, not Python, because
// the table tracks whichever Unicode version Node ships (Python 3.13 is a version behind and misses
// 37 characters):
//
//	for (let cp = 0x80; cp <= 0x10FFFF; cp++) {           // skipping U+D800-U+DFFF
//	  const c = String.fromCodePoint(cp);
//	  const d = c.normalize('NFKD').replace(/\p{Mn}/gu, '').toLowerCase();
//	  // collapse each run of non-[a-z0-9] in d to a single '-', keep c when the result differs
//	  // from c and either holds ASCII alphanumerics or is empty, and group the characters by that
//	  // result
//	}

// slugFoldGroups maps a folded spelling to the characters that fold onto it.
var slugFoldGroups = []struct {
	fold  string
	chars string
}{
	// Folds to nothing. The reference decomposes before it strips marks, so a character whose NFKD
	// is nothing but marks disappears there rather than becoming a separator: U+FF9E decomposes to
	// U+3099, a non-spacing mark, which the strip then removes. These two are the only characters in
	// Unicode that are not marks themselves and still vanish; without the entry, a name containing
	// one would slug with a hyphen here and without one in every other SDK.
	{"", "\uFF9E\uFF9F"},
	{"0", "⁰₀⓪０𜳰𝟎𝟘𝟢𝟬𝟶🯰"},
	{"1", "¹₁①１𜳱𝟏𝟙𝟣𝟭𝟷🯱"},
	{"2", "²₂②２𜳲𝟐𝟚𝟤𝟮𝟸🯲"},
	{"3", "³₃③３𜳳𝟑𝟛𝟥𝟯𝟹🯳"},
	{"4", "⁴₄④４𜳴𝟒𝟜𝟦𝟰𝟺🯴"},
	{"5", "⁵₅⑤５𜳵𝟓𝟝𝟧𝟱𝟻🯵"},
	{"6", "⁶₆⑥６𜳶𝟔𝟞𝟨𝟲𝟼🯶"},
	{"7", "⁷₇⑦７𜳷𝟕𝟟𝟩𝟳𝟽🯷"},
	{"8", "⁸₈⑧８𜳸𝟖𝟠𝟪𝟴𝟾🯸"},
	{"9", "⁹₉⑨９𜳹𝟗𝟡𝟫𝟵𝟿🯹"},
	{"a", "ªÀÁÂÃÄÅàáâãäåĀāĂăĄąǍǎǞǟǠǡǺǻȀȁȂȃȦȧᴬᵃḀḁẠạẢảẤấẦầẨẩẪẫẬậẮắẰằẲẳẴẵẶặₐÅⒶⓐＡａ𜳖𝐀𝐚𝐴𝑎𝑨𝒂𝒜𝒶𝓐𝓪𝔄𝔞𝔸𝕒𝕬𝖆𝖠𝖺𝗔𝗮𝘈𝘢𝘼𝙖𝙰𝚊🄰"},
	{"b", "ᴮᵇḂḃḄḅḆḇℬⒷⓑＢｂ𜳗𝐁𝐛𝐵𝑏𝑩𝒃𝒷𝓑𝓫𝔅𝔟𝔹𝕓𝕭𝖇𝖡𝖻𝗕𝗯𝘉𝘣𝘽𝙗𝙱𝚋🄱"},
	{"c", "ÇçĆćĈĉĊċČčᶜḈḉℂℭⅭⅽⒸⓒꟲＣｃ𜳘𝐂𝐜𝐶𝑐𝑪𝒄𝒞𝒸𝓒𝓬𝔠𝕔𝕮𝖈𝖢𝖼𝗖𝗰𝘊𝘤𝘾𝙘𝙲𝚌🄫🄲"},
	{"d", "ĎďᴰᵈḊḋḌḍḎḏḐḑḒḓⅅⅆⅮⅾⒹⓓＤｄ𜳙𝐃𝐝𝐷𝑑𝑫𝒅𝒟𝒹𝓓𝓭𝔇𝔡𝔻𝕕𝕯𝖉𝖣𝖽𝗗𝗱𝘋𝘥𝘿𝙙𝙳𝚍🄳"},
	{"e", "ÈÉÊËèéêëĒēĔĕĖėĘęĚěȄȅȆȇȨȩᴱᵉḔḕḖḗḘḙḚḛḜḝẸẹẺẻẼẽẾếỀềỂểỄễỆệₑℯℰⅇⒺⓔＥｅ𜳚𝐄𝐞𝐸𝑒𝑬𝒆𝓔𝓮𝔈𝔢𝔼𝕖𝕰𝖊𝖤𝖾𝗘𝗲𝘌𝘦𝙀𝙚𝙴𝚎🄴"},
	{"f", "ᶠḞḟℱⒻⓕꟳＦｆ𜳛𝐅𝐟𝐹𝑓𝑭𝒇𝒻𝓕𝓯𝔉𝔣𝔽𝕗𝕱𝖋𝖥𝖿𝗙𝗳𝘍𝘧𝙁𝙛𝙵𝚏🄵"},
	{"g", "ĜĝĞğĠġĢģǦǧǴǵᴳᵍḠḡℊⒼⓖＧｇ𜳜𝐆𝐠𝐺𝑔𝑮𝒈𝒢𝓖𝓰𝔊𝔤𝔾𝕘𝕲𝖌𝖦𝗀𝗚𝗴𝘎𝘨𝙂𝙜𝙶𝚐🄶"},
	{"h", "ĤĥȞȟʰᴴḢḣḤḥḦḧḨḩḪḫẖₕℋℌℍℎⒽⓗＨｈ𜳝𝐇𝐡𝐻𝑯𝒉𝒽𝓗𝓱𝔥𝕙𝕳𝖍𝖧𝗁𝗛𝗵𝘏𝘩𝙃𝙝𝙷𝚑🄷"},
	{"i", "ÌÍÎÏìíîïĨĩĪīĬĭĮįİǏǐȈȉȊȋᴵᵢḬḭḮḯỈỉỊịⁱℐℑℹⅈⅠⅰⒾⓘＩｉ𜳞𝐈𝐢𝐼𝑖𝑰𝒊𝒾𝓘𝓲𝔦𝕀𝕚𝕴𝖎𝖨𝗂𝗜𝗶𝘐𝘪𝙄𝙞𝙸𝚒🄸"},
	{"j", "ĴĵǰʲᴶⅉⒿⓙⱼＪｊ𜳟𝐉𝐣𝐽𝑗𝑱𝒋𝒥𝒿𝓙𝓳𝔍𝔧𝕁𝕛𝕵𝖏𝖩𝗃𝗝𝗷𝘑𝘫𝙅𝙟𝙹𝚓🄹"},
	{"k", "ĶķǨǩᴷᵏḰḱḲḳḴḵₖⓀⓚＫｋ𜳠𝐊𝐤𝐾𝑘𝑲𝒌𝒦𝓀𝓚𝓴𝔎𝔨𝕂𝕜𝕶𝖐𝖪𝗄𝗞𝗸𝘒𝘬𝙆𝙠𝙺𝚔🄺"},
	{"l", "ĹĺĻļĽľˡᴸḶḷḸḹḺḻḼḽₗℒℓⅬⅼⓁⓛＬｌ𜳡𝐋𝐥𝐿𝑙𝑳𝒍𝓁𝓛𝓵𝔏𝔩𝕃𝕝𝕷𝖑𝖫𝗅𝗟𝗹𝘓𝘭𝙇𝙡𝙻𝚕🄻"},
	{"m", "ᴹᵐḾḿṀṁṂṃₘℳⅯⅿⓂⓜＭｍ𜳢𝐌𝐦𝑀𝑚𝑴𝒎𝓂𝓜𝓶𝔐𝔪𝕄𝕞𝕸𝖒𝖬𝗆𝗠𝗺𝘔𝘮𝙈𝙢𝙼𝚖🄼"},
	{"n", "ÑñŃńŅņŇňǸǹᴺṄṅṆṇṈṉṊṋⁿₙℕⓃⓝＮｎ𜳣𝐍𝐧𝑁𝑛𝑵𝒏𝒩𝓃𝓝𝓷𝔑𝔫𝕟𝕹𝖓𝖭𝗇𝗡𝗻𝘕𝘯𝙉𝙣𝙽𝚗🄽"},
	{"o", "ºÒÓÔÕÖòóôõöŌōŎŏŐőƠơǑǒǪǫǬǭȌȍȎȏȪȫȬȭȮȯȰȱᴼᵒṌṍṎṏṐṑṒṓỌọỎỏỐốỒồỔổỖỗỘộỚớỜờỞởỠỡỢợₒℴⓄⓞＯｏ𜳤𝐎𝐨𝑂𝑜𝑶𝒐𝒪𝓞𝓸𝔒𝔬𝕆𝕠𝕺𝖔𝖮𝗈𝗢𝗼𝘖𝘰𝙊𝙤𝙾𝚘🄾"},
	{"p", "ᴾᵖṔṕṖṗₚℙⓅⓟＰｐ𜳥𝐏𝐩𝑃𝑝𝑷𝒑𝒫𝓅𝓟𝓹𝔓𝔭𝕡𝕻𝖕𝖯𝗉𝗣𝗽𝘗𝘱𝙋𝙥𝙿𝚙🄿"},
	{"q", "ℚⓆⓠꟴＱｑ𐞥𜳦𝐐𝐪𝑄𝑞𝑸𝒒𝒬𝓆𝓠𝓺𝔔𝔮𝕢𝕼𝖖𝖰𝗊𝗤𝗾𝘘𝘲𝙌𝙦𝚀𝚚🅀"},
	{"r", "ŔŕŖŗŘřȐȑȒȓʳᴿᵣṘṙṚṛṜṝṞṟℛℜℝⓇⓡＲｒ𜳧𝐑𝐫𝑅𝑟𝑹𝒓𝓇𝓡𝓻𝔯𝕣𝕽𝖗𝖱𝗋𝗥𝗿𝘙𝘳𝙍𝙧𝚁𝚛🄬🅁"},
	{"s", "ŚśŜŝŞşŠšſȘșˢṠṡṢṣṤṥṦṧṨṩẛₛⓈⓢ꟱Ｓｓ𜳨𝐒𝐬𝑆𝑠𝑺𝒔𝒮𝓈𝓢𝓼𝔖𝔰𝕊𝕤𝕾𝖘𝖲𝗌𝗦𝘀𝘚𝘴𝙎𝙨𝚂𝚜🅂"},
	{"t", "ŢţŤťȚțᵀᵗṪṫṬṭṮṯṰṱẗₜⓉⓣＴｔ𜳩𝐓𝐭𝑇𝑡𝑻𝒕𝒯𝓉𝓣𝓽𝔗𝔱𝕋𝕥𝕿𝖙𝖳𝗍𝗧𝘁𝘛𝘵𝙏𝙩𝚃𝚝🅃"},
	{"u", "ÙÚÛÜùúûüŨũŪūŬŭŮůŰűŲųƯưǓǔǕǖǗǘǙǚǛǜȔȕȖȗᵁᵘᵤṲṳṴṵṶṷṸṹṺṻỤụỦủỨứỪừỬửỮữỰựⓊⓤＵｕ𜳪𝐔𝐮𝑈𝑢𝑼𝒖𝒰𝓊𝓤𝓾𝔘𝔲𝕌𝕦𝖀𝖚𝖴𝗎𝗨𝘂𝘜𝘶𝙐𝙪𝚄𝚞🅄"},
	{"v", "ᵛᵥṼṽṾṿⅤⅴⓋⓥⱽＶｖ𜳫𝐕𝐯𝑉𝑣𝑽𝒗𝒱𝓋𝓥𝓿𝔙𝔳𝕍𝕧𝖁𝖛𝖵𝗏𝗩𝘃𝘝𝘷𝙑𝙫𝚅𝚟🅅"},
	{"w", "ŴŵʷᵂẀẁẂẃẄẅẆẇẈẉẘⓌⓦＷｗ𜳬𝐖𝐰𝑊𝑤𝑾𝒘𝒲𝓌𝓦𝔀𝔚𝔴𝕎𝕨𝖂𝖜𝖶𝗐𝗪𝘄𝘞𝘸𝙒𝙬𝚆𝚠🅆"},
	{"x", "ˣẊẋẌẍₓⅩⅹⓍⓧＸｘ𜳭𝐗𝐱𝑋𝑥𝑿𝒙𝒳𝓍𝓧𝔁𝔛𝔵𝕏𝕩𝖃𝖝𝖷𝗑𝗫𝘅𝘟𝘹𝙓𝙭𝚇𝚡🅇"},
	{"y", "ÝýÿŶŷŸȲȳʸẎẏẙỲỳỴỵỶỷỸỹⓎⓨＹｙ𜳮𝐘𝐲𝑌𝑦𝒀𝒚𝒴𝓎𝓨𝔂𝔜𝔶𝕐𝕪𝖄𝖞𝖸𝗒𝗬𝘆𝘠𝘺𝙔𝙮𝚈𝚢🅈"},
	{"z", "ŹźŻżŽžᶻẐẑẒẓẔẕℤℨⓏⓩＺｚ𜳯𝐙𝐳𝑍𝑧𝒁𝒛𝒵𝓏𝓩𝔃𝔷𝕫𝖅𝖟𝖹𝗓𝗭𝘇𝘡𝘻𝙕𝙯𝚉𝚣🅉"},
	{"-a", "㎂"},
	{"-c", "℃"},
	{"-f", "℉㎌"},
	{"-g", "㎍"},
	{"-l", "㎕"},
	{"-m", "㎛"},
	{"-n", "ŉ"},
	{"-s", "㎲"},
	{"-v", "㎶"},
	{"-w", "㎼"},
	{"0-", "㍘🄀🄁"},
	{"1-", "⅟⒈㋀㍙㏠🄂"},
	{"10", "⑩"},
	{"11", "⑪"},
	{"12", "⑫"},
	{"13", "⑬"},
	{"14", "⑭"},
	{"15", "⑮"},
	{"16", "⑯"},
	{"17", "⑰"},
	{"18", "⑱"},
	{"19", "⑲"},
	{"2-", "⒉㋁㍚㏡🄃"},
	{"20", "⑳"},
	{"21", "㉑"},
	{"22", "㉒"},
	{"23", "㉓"},
	{"24", "㉔"},
	{"25", "㉕"},
	{"26", "㉖"},
	{"27", "㉗"},
	{"28", "㉘"},
	{"29", "㉙"},
	{"3-", "⒊㋂㍛㏢🄄"},
	{"30", "㉚"},
	{"31", "㉛"},
	{"32", "㉜"},
	{"33", "㉝"},
	{"34", "㉞"},
	{"35", "㉟"},
	{"36", "㊱"},
	{"37", "㊲"},
	{"38", "㊳"},
	{"39", "㊴"},
	{"4-", "⒋㋃㍜㏣🄅"},
	{"40", "㊵"},
	{"41", "㊶"},
	{"42", "㊷"},
	{"43", "㊸"},
	{"44", "㊹"},
	{"45", "㊺"},
	{"46", "㊻"},
	{"47", "㊼"},
	{"48", "㊽"},
	{"49", "㊾"},
	{"5-", "⒌㋄㍝㏤🄆"},
	{"50", "㊿"},
	{"6-", "⒍㋅㍞㏥🄇"},
	{"7-", "⒎㋆㍟㏦🄈"},
	{"8-", "⒏㋇㍠㏧🄉"},
	{"9-", "⒐㋈㍡㏨🄊"},
	{"a-", "ẚ"},
	{"au", "㍳"},
	{"bq", "㏃"},
	{"cc", "㏄"},
	{"cd", "㏅🄭"},
	{"cm", "㎝"},
	{"da", "㍲"},
	{"db", "㏈"},
	{"dj", "🆐"},
	{"dl", "㎗"},
	{"dm", "㍷"},
	{"dz", "ǄǅǆǱǲǳ"},
	{"ev", "㋎"},
	{"ff", "ﬀ"},
	{"fi", "ﬁ"},
	{"fl", "ﬂ"},
	{"fm", "㎙"},
	{"gb", "㎇"},
	{"gy", "㏉"},
	{"ha", "㏊"},
	{"hg", "㋌"},
	{"hp", "㏋"},
	{"hv", "🅊"},
	{"hz", "㎐"},
	{"ii", "Ⅱⅱ"},
	{"ij", "Ĳĳ"},
	{"in", "㏌"},
	{"iu", "㍺"},
	{"iv", "Ⅳⅳ"},
	{"ix", "Ⅸⅸ"},
	{"k-", "㏀"},
	{"ka", "㎄"},
	{"kb", "㎅"},
	{"kg", "㎏"},
	{"kk", "㏍"},
	{"kl", "㎘"},
	{"km", "㎞㏎"},
	{"kt", "㏏"},
	{"kv", "㎸"},
	{"kw", "㎾"},
	{"l-", "Ŀŀ"},
	{"lj", "Ǉǈǉ"},
	{"lm", "㏐"},
	{"ln", "㏑"},
	{"lx", "㏓"},
	{"m-", "㏁"},
	{"m2", "㎡"},
	{"m3", "㎥"},
	{"ma", "㎃"},
	{"mb", "㎆㏔"},
	{"mc", "🅪"},
	{"md", "🅫"},
	{"mg", "㎎"},
	{"ml", "㎖"},
	{"mm", "㎜"},
	{"mr", "🅬"},
	{"ms", "㎳"},
	{"mv", "㎷㎹🅋"},
	{"mw", "㎽㎿"},
	{"na", "㎁"},
	{"nf", "㎋"},
	{"nj", "Ǌǋǌ"},
	{"nm", "㎚"},
	{"no", "№"},
	{"ns", "㎱"},
	{"nv", "㎵"},
	{"nw", "㎻"},
	{"ov", "㍵"},
	{"pa", "㎀㎩"},
	{"pc", "㍶"},
	{"pf", "㎊"},
	{"ph", "㏗"},
	{"pr", "㏚"},
	{"ps", "㎰"},
	{"pv", "㎴"},
	{"pw", "㎺"},
	{"rs", "₨"},
	{"sd", "🅌"},
	{"sm", "℠"},
	{"sr", "㏛"},
	{"ss", "🅍"},
	{"st", "ﬅﬆ"},
	{"sv", "㏜"},
	{"tm", "™"},
	{"vi", "Ⅵⅵ"},
	{"wb", "㏝"},
	{"wc", "🅏"},
	{"wz", "🄮"},
	{"xi", "Ⅺⅺ"},
	{"-1-", "⑴"},
	{"-2-", "⑵"},
	{"-3-", "⑶"},
	{"-4-", "⑷"},
	{"-5-", "⑸"},
	{"-6-", "⑹"},
	{"-7-", "⑺"},
	{"-8-", "⑻"},
	{"-9-", "⑼"},
	{"-a-", "⒜🄐"},
	{"-b-", "⒝🄑"},
	{"-c-", "⒞🄒"},
	{"-d-", "⒟🄓"},
	{"-e-", "⒠🄔"},
	{"-f-", "⒡🄕"},
	{"-g-", "⒢🄖"},
	{"-h-", "⒣🄗"},
	{"-i-", "⒤🄘"},
	{"-j-", "⒥🄙"},
	{"-k-", "⒦🄚"},
	{"-l-", "⒧🄛"},
	{"-m-", "⒨🄜"},
	{"-n-", "⒩🄝"},
	{"-o-", "⒪🄞"},
	{"-p-", "⒫🄟"},
	{"-q-", "⒬🄠"},
	{"-r-", "⒭🄡"},
	{"-s-", "⒮🄢🄪"},
	{"-t-", "⒯🄣"},
	{"-u-", "⒰🄤"},
	{"-v-", "⒱🄥"},
	{"-w-", "⒲🄦"},
	{"-x-", "⒳🄧"},
	{"-y-", "⒴🄨"},
	{"-z-", "⒵🄩"},
	{"0-3", "↉"},
	{"1-2", "½"},
	{"1-3", "⅓"},
	{"1-4", "¼"},
	{"1-5", "⅕"},
	{"1-6", "⅙"},
	{"1-7", "⅐"},
	{"1-8", "⅛"},
	{"1-9", "⅑"},
	{"10-", "⒑㋉㍢㏩"},
	{"11-", "⒒㋊㍣㏪"},
	{"12-", "⒓㋋㍤㏫"},
	{"13-", "⒔㍥㏬"},
	{"14-", "⒕㍦㏭"},
	{"15-", "⒖㍧㏮"},
	{"16-", "⒗㍨㏯"},
	{"17-", "⒘㍩㏰"},
	{"18-", "⒙㍪㏱"},
	{"19-", "⒚㍫㏲"},
	{"2-3", "⅔"},
	{"2-5", "⅖"},
	{"20-", "⒛㍬㏳"},
	{"21-", "㍭㏴"},
	{"22-", "㍮㏵"},
	{"23-", "㍯㏶"},
	{"24-", "㍰㏷"},
	{"25-", "㏸"},
	{"26-", "㏹"},
	{"27-", "㏺"},
	{"28-", "㏻"},
	{"29-", "㏼"},
	{"3-4", "¾"},
	{"3-5", "⅗"},
	{"3-8", "⅜"},
	{"30-", "㏽"},
	{"31-", "㏾"},
	{"4-5", "⅘"},
	{"5-6", "⅚"},
	{"5-8", "⅝"},
	{"7-8", "⅞"},
	{"a-c", "℀"},
	{"a-m", "㏟"},
	{"a-s", "℁"},
	{"bar", "㍴"},
	{"c-o", "℅"},
	{"c-u", "℆"},
	{"cal", "㎈"},
	{"cm2", "㎠"},
	{"cm3", "㎤"},
	{"co-", "㏇"},
	{"dm2", "㍸"},
	{"dm3", "㍹"},
	{"erg", "㋍"},
	{"fax", "℻"},
	{"ffi", "ﬃ"},
	{"ffl", "ﬄ"},
	{"gal", "㏿"},
	{"ghz", "㎓"},
	{"gpa", "㎬"},
	{"hpa", "㍱"},
	{"iii", "Ⅲⅲ"},
	{"khz", "㎑"},
	{"km2", "㎢"},
	{"km3", "㎦"},
	{"kpa", "㎪"},
	{"log", "㏒"},
	{"ltd", "㋏"},
	{"m-s", "㎧"},
	{"mhz", "㎒"},
	{"mil", "㏕"},
	{"mm2", "㎟"},
	{"mm3", "㎣"},
	{"mol", "㏖"},
	{"mpa", "㎫"},
	{"ppm", "㏙"},
	{"ppv", "🅎"},
	{"pte", "㉐"},
	{"rad", "㎭"},
	{"tel", "℡"},
	{"thz", "㎔"},
	{"v-m", "㏞"},
	{"vii", "Ⅶⅶ"},
	{"xii", "Ⅻⅻ"},
	{"-10-", "⑽"},
	{"-11-", "⑾"},
	{"-12-", "⑿"},
	{"-13-", "⒀"},
	{"-14-", "⒁"},
	{"-15-", "⒂"},
	{"-16-", "⒃"},
	{"-17-", "⒄"},
	{"-18-", "⒅"},
	{"-19-", "⒆"},
	{"-20-", "⒇"},
	{"1-10", "⅒"},
	{"a-m-", "㏂"},
	{"c-kg", "㏆"},
	{"kcal", "㎉"},
	{"m-s2", "㎨"},
	{"p-m-", "㏘"},
	{"viii", "Ⅷⅷ"},
	{"rad-s", "㎮"},
	{"rad-s2", "㎯"},
}

// slugFolds is slugFoldGroups keyed by character, built once at package load.
var slugFolds = buildSlugFolds()

func buildSlugFolds() map[rune]string {
	folds := make(map[rune]string, 2048)
	for _, group := range slugFoldGroups {
		for _, r := range group.chars {
			folds[r] = group.fold
		}
	}
	return folds
}

// slugMarkAdditions are the non-spacing marks that the reference engine's \p{Mn} matches and this
// toolchain's unicode.Mn does not. Generated data — the same recipe as above, comparing the two:
//
//	/\p{Mn}/u.test(String.fromCodePoint(cp))    // node, against unicode.Is(unicode.Mn, cp) in Go
//
// Every SDK reads the Mn category from its own runtime, so Go does too; the runtimes are just not
// all on the same Unicode version at the same time. Go 1.21 and 1.22 carry Unicode 15.0 (see
// unicode.Version), and these 75 marks were encoded later. The list is a no-op once this
// toolchain's tables catch up, so it is safe to leave in place and safe to delete.
var slugMarkAdditions = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x0897, 0x0897, 1},
		{0x1acf, 0x1add, 1},
		{0x1ae0, 0x1aeb, 1},
	},
	R32: []unicode.Range32{
		{0x10d69, 0x10d6d, 1},
		{0x10efa, 0x10efc, 1},
		{0x113bb, 0x113c0, 1},
		{0x113ce, 0x113ce, 1},
		{0x113d0, 0x113d0, 1},
		{0x113d2, 0x113d2, 1},
		{0x113e1, 0x113e2, 1},
		{0x11b60, 0x11b60, 1},
		{0x11b62, 0x11b64, 1},
		{0x11b66, 0x11b66, 1},
		{0x11f5a, 0x11f5a, 1},
		{0x1611e, 0x16129, 1},
		{0x1612d, 0x1612f, 1},
		{0x1e5ee, 0x1e5ef, 1},
		{0x1e6e3, 0x1e6e3, 1},
		{0x1e6e6, 0x1e6e6, 1},
		{0x1e6ee, 0x1e6ef, 1},
		{0x1e6f5, 0x1e6f5, 1},
	},
}

// slugMarkReclassified is U+1171E, AHOM CONSONANT SIGN MEDIAL RA. It is the one correction in the
// other direction: Unicode 15.0, which this toolchain's unicode.Mn follows, files it as a
// non-spacing mark, and Unicode 15.1 moved it to Mc, so the reference engine does not strip it and
// neither may this. Also a no-op once the toolchain's tables catch up.
const slugMarkReclassified = '\U0001171E'

// isSlugMark reports whether r is a non-spacing mark that deriveDestinationSlug drops, matching
// what /\p{Mn}/gu matches in the reference implementation.
func isSlugMark(r rune) bool {
	if r == slugMarkReclassified {
		return false
	}
	return unicode.Is(unicode.Mn, r) || unicode.Is(slugMarkAdditions, r)
}
